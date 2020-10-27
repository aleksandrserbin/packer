package packer

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
)

func TestProvisionHook_Impl(t *testing.T) {
	var raw interface{}
	raw = &ProvisionHook{}
	if _, ok := raw.(Hook); !ok {
		t.Fatalf("must be a Hook")
	}
}

func TestProvisionHook(t *testing.T) {
	pA := &MockProvisioner{}
	pB := &MockProvisioner{}

	ui := testUi()
	var comm Communicator = new(MockCommunicator)
	var data interface{} = nil

	hook := &ProvisionHook{
		Provisioners: []*HookedProvisioner{
			{pA, nil, ""},
			{pB, nil, ""},
		},
	}

	hook.Run(context.Background(), "foo", ui, comm, data)

	if !pA.ProvCalled {
		t.Error("provision should be called on pA")
	}

	if !pB.ProvCalled {
		t.Error("provision should be called on pB")
	}
}

func TestProvisionHook_nilComm(t *testing.T) {
	pA := &MockProvisioner{}
	pB := &MockProvisioner{}

	ui := testUi()
	var comm Communicator = nil
	var data interface{} = nil

	hook := &ProvisionHook{
		Provisioners: []*HookedProvisioner{
			{pA, nil, ""},
			{pB, nil, ""},
		},
	}

	err := hook.Run(context.Background(), "foo", ui, comm, data)
	if err == nil {
		t.Fatal("should error")
	}
}

func TestProvisionHook_cancel(t *testing.T) {
	topCtx, topCtxCancel := context.WithCancel(context.Background())

	p := &MockProvisioner{
		ProvFunc: func(ctx context.Context) error {
			topCtxCancel()
			<-ctx.Done()
			return ctx.Err()
		},
	}

	hook := &ProvisionHook{
		Provisioners: []*HookedProvisioner{
			{p, nil, ""},
		},
	}

	err := hook.Run(topCtx, "foo", nil, new(MockCommunicator), nil)
	if err == nil {
		t.Fatal("should have err")
	}
}

// TODO(mitchellh): Test that they're run in the proper order

func TestPausedProvisioner_impl(t *testing.T) {
	var _ Provisioner = new(PausedProvisioner)
}

func TestPausedProvisionerPrepare(t *testing.T) {
	mock := new(MockProvisioner)
	prov := &PausedProvisioner{
		Provisioner: mock,
	}

	prov.Prepare(42)
	if !mock.PrepCalled {
		t.Fatal("prepare should be called")
	}
	if mock.PrepConfigs[0] != 42 {
		t.Fatal("should have proper configs")
	}
}

func TestPausedProvisionerProvision(t *testing.T) {
	mock := new(MockProvisioner)
	prov := &PausedProvisioner{
		Provisioner: mock,
	}

	ui := testUi()
	comm := new(MockCommunicator)
	prov.Provision(context.Background(), ui, comm, make(map[string]interface{}))
	if !mock.ProvCalled {
		t.Fatal("prov should be called")
	}
	if mock.ProvUi != ui {
		t.Fatal("should have proper ui")
	}
	if mock.ProvCommunicator != comm {
		t.Fatal("should have proper comm")
	}
}

func TestPausedProvisionerProvision_waits(t *testing.T) {
	startTime := time.Now()
	waitTime := 50 * time.Millisecond

	prov := &PausedProvisioner{
		PauseBefore: waitTime,
		Provisioner: &MockProvisioner{
			ProvFunc: func(context.Context) error {
				timeSinceStartTime := time.Since(startTime)
				if timeSinceStartTime < waitTime {
					return fmt.Errorf("Spent not enough time waiting: %s", timeSinceStartTime)
				}
				return nil
			},
		},
	}

	err := prov.Provision(context.Background(), testUi(), new(MockCommunicator), make(map[string]interface{}))

	if err != nil {
		t.Fatalf("prov failed: %v", err)
	}
}

func TestPausedProvisionerCancel(t *testing.T) {
	topCtx, cancelTopCtx := context.WithCancel(context.Background())

	mock := new(MockProvisioner)
	prov := &PausedProvisioner{
		Provisioner: mock,
	}

	mock.ProvFunc = func(ctx context.Context) error {
		cancelTopCtx()
		<-ctx.Done()
		return ctx.Err()
	}

	err := prov.Provision(topCtx, testUi(), new(MockCommunicator), make(map[string]interface{}))
	if err == nil {
		t.Fatal("should have err")
	}
}

func TestDebuggedProvisioner_impl(t *testing.T) {
	var _ Provisioner = new(DebuggedProvisioner)
}

func TestDebuggedProvisionerPrepare(t *testing.T) {
	mock := new(MockProvisioner)
	prov := &DebuggedProvisioner{
		Provisioner: mock,
	}

	prov.Prepare(42)
	if !mock.PrepCalled {
		t.Fatal("prepare should be called")
	}
	if mock.PrepConfigs[0] != 42 {
		t.Fatal("should have proper configs")
	}
}

func TestDebuggedProvisionerProvision(t *testing.T) {
	mock := new(MockProvisioner)
	prov := &DebuggedProvisioner{
		Provisioner: mock,
	}

	ui := testUi()
	comm := new(MockCommunicator)
	writeReader(ui, "\n")
	prov.Provision(context.Background(), ui, comm, make(map[string]interface{}))
	if !mock.ProvCalled {
		t.Fatal("prov should be called")
	}
	if mock.ProvUi != ui {
		t.Fatal("should have proper ui")
	}
	if mock.ProvCommunicator != comm {
		t.Fatal("should have proper comm")
	}
}

func TestDebuggedProvisionerCancel(t *testing.T) {
	topCtx, topCtxCancel := context.WithCancel(context.Background())

	mock := new(MockProvisioner)
	prov := &DebuggedProvisioner{
		Provisioner: mock,
	}

	mock.ProvFunc = func(ctx context.Context) error {
		topCtxCancel()
		<-ctx.Done()
		return ctx.Err()
	}

	err := prov.Provision(topCtx, testUi(), new(MockCommunicator), make(map[string]interface{}))
	if err == nil {
		t.Fatal("should have error")
	}
}

func TestRetriedProvisioner_impl(t *testing.T) {
	var _ Provisioner = new(RetriedProvisioner)
}

func TestRetriedProvisionerPrepare(t *testing.T) {
	mock := new(MockProvisioner)
	prov := &RetriedProvisioner{
		Provisioner: mock,
	}

	err := prov.Prepare(42)
	if err != nil {
		t.Fatal("should not have errored")
	}
	if !mock.PrepCalled {
		t.Fatal("prepare should be called")
	}
	if mock.PrepConfigs[0] != 42 {
		t.Fatal("should have proper configs")
	}
}

func TestRetriedProvisionerProvision(t *testing.T) {
	mock := &MockProvisioner{
		ProvFunc: func(ctx context.Context) error {
			return errors.New("failed")
		},
	}

	prov := &RetriedProvisioner{
		MaxRetries:  2,
		Provisioner: mock,
	}

	ui := testUi()
	comm := new(MockCommunicator)
	err := prov.Provision(context.Background(), ui, comm, make(map[string]interface{}))
	if err != nil {
		t.Fatal("should not have errored")
	}
	if !mock.ProvCalled {
		t.Fatal("prov should be called")
	}
	if !mock.ProvRetried {
		t.Fatal("prov should be retried")
	}
	if mock.ProvUi != ui {
		t.Fatal("should have proper ui")
	}
	if mock.ProvCommunicator != comm {
		t.Fatal("should have proper comm")
	}
}

func TestRetriedProvisionerCancelledProvision(t *testing.T) {
	// Don't retry if context is cancelled
	ctx, topCtxCancel := context.WithCancel(context.Background())

	mock := &MockProvisioner{
		ProvFunc: func(ctx context.Context) error {
			topCtxCancel()
			<-ctx.Done()
			return ctx.Err()
		},
	}

	prov := &RetriedProvisioner{
		MaxRetries:  2,
		Provisioner: mock,
	}

	ui := testUi()
	comm := new(MockCommunicator)
	err := prov.Provision(ctx, ui, comm, make(map[string]interface{}))
	if err == nil {
		t.Fatal("should have errored")
	}
	if !mock.ProvCalled {
		t.Fatal("prov should be called")
	}
	if mock.ProvRetried {
		t.Fatal("prov should NOT be retried")
	}
	if mock.ProvUi != ui {
		t.Fatal("should have proper ui")
	}
	if mock.ProvCommunicator != comm {
		t.Fatal("should have proper comm")
	}
}

func TestRetriedProvisionerCancel(t *testing.T) {
	topCtx, cancelTopCtx := context.WithCancel(context.Background())

	mock := new(MockProvisioner)
	prov := &RetriedProvisioner{
		Provisioner: mock,
	}

	mock.ProvFunc = func(ctx context.Context) error {
		cancelTopCtx()
		<-ctx.Done()
		return ctx.Err()
	}

	err := prov.Provision(topCtx, testUi(), new(MockCommunicator), make(map[string]interface{}))
	if err == nil {
		t.Fatal("should have err")
	}
}

func TestNamedProvisioner_impl(t *testing.T) {
	var _ Provisioner = new(NamedProvisioner)
}

func TestNamedProvisionerPrepare(t *testing.T) {
	mock := new(MockProvisioner)
	prov := &NamedProvisioner{
		Provisioner: mock,
	}

	err := prov.Prepare(1, 2, 3)
	if err != nil {
		t.Fatal("should not have errored")
	}
	if !mock.PrepCalled {
		t.Fatal("prepare should be called")
	}
	if !cmp.Equal(mock.PrepConfigs, []interface{}{1, 2, 3}) {
		t.Fatal("should have proper configs")
	}
}

func TestNamedProvisionerProvision(t *testing.T) {
	mock := new(MockProvisioner)
	prov := &NamedProvisioner{
		Name:        "testing",
		Provisioner: mock,
	}

	ui := testUi()
	comm := new(MockCommunicator)
	err := prov.Provision(context.Background(), ui, comm, make(map[string]interface{}))
	if err != nil {
		t.Fatal("should not have errored")
	}
	if !mock.ProvCalled {
		t.Fatal("prov should be called")
	}
	if mock.ProvUi != ui {
		t.Fatal("should have proper ui")
	}
	if mock.ProvCommunicator != comm {
		t.Fatal("should have proper comm")
	}
	var output = readWriter(ui)
	if !strings.Contains(output, "Starting provisioner testing") {
		t.Fatal("should have printed provisioner name")
	}
}
