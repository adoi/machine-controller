/*
Copyright 2026 The Machine Controller Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package kubevirt

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	kubevirtcorev1 "kubevirt.io/api/core/v1"
	kubevirtinstancetypev1alpha1 "kubevirt.io/api/instancetype/v1alpha1"
	kubevirtinstancetypev1beta1 "kubevirt.io/api/instancetype/v1beta1"

	cloudprovidertesting "k8c.io/machine-controller/pkg/cloudprovider/testing"
	"k8c.io/machine-controller/sdk/providerconfig/configvar"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	ctrlruntimeclient "sigs.k8s.io/controller-runtime/pkg/client"
	fakectrlruntimeclient "sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestDefaultVMInstanceType(t *testing.T) {
	const namespace = "tenant-a"

	namespaced := func(name string) ctrlruntimeclient.Object {
		return &kubevirtinstancetypev1beta1.VirtualMachineInstancetype{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
		}
	}
	cluster := func(name string) ctrlruntimeclient.Object {
		return &kubevirtinstancetypev1beta1.VirtualMachineClusterInstancetype{
			ObjectMeta: metav1.ObjectMeta{Name: name},
		}
	}

	tests := []struct {
		name      string
		matcher   *kubevirtcorev1.InstancetypeMatcher
		namespace string
		objects   []ctrlruntimeclient.Object
		want      *kubevirtcorev1.InstancetypeMatcher
		wantErr   string
	}{
		{
			name:      "nil matcher",
			namespace: namespace,
		},
		{
			name: "explicit namespaced kind wins when names collide",
			matcher: &kubevirtcorev1.InstancetypeMatcher{
				Name: "shared",
				Kind: "VirtualMachineInstancetype",
			},
			namespace: namespace,
			objects:   []ctrlruntimeclient.Object{namespaced("shared"), cluster("shared")},
			want: &kubevirtcorev1.InstancetypeMatcher{
				Name: "shared",
				Kind: "VirtualMachineInstancetype",
			},
		},
		{
			name: "explicit cluster kind wins when names collide",
			matcher: &kubevirtcorev1.InstancetypeMatcher{
				Name: "shared",
				Kind: "VirtualMachineClusterInstancetype",
			},
			namespace: namespace,
			objects:   []ctrlruntimeclient.Object{namespaced("shared"), cluster("shared")},
			want: &kubevirtcorev1.InstancetypeMatcher{
				Name: "shared",
				Kind: "VirtualMachineClusterInstancetype",
			},
		},
		{
			name: "explicit namespaced kind does not fall back to cluster scope",
			matcher: &kubevirtcorev1.InstancetypeMatcher{
				Name: "cluster-only",
				Kind: "VirtualMachineInstancetype",
			},
			namespace: namespace,
			objects:   []ctrlruntimeclient.Object{cluster("cluster-only")},
			wantErr:   `VirtualMachineInstancetype "cluster-only" not found`,
		},
		{
			name: "explicit cluster kind does not fall back to namespace scope",
			matcher: &kubevirtcorev1.InstancetypeMatcher{
				Name: "namespaced-only",
				Kind: "VirtualMachineClusterInstancetype",
			},
			namespace: namespace,
			objects:   []ctrlruntimeclient.Object{namespaced("namespaced-only")},
			wantErr:   `VirtualMachineClusterInstancetype "namespaced-only" not found`,
		},
		{
			name: "legacy empty kind uses namespaced-first discovery",
			matcher: &kubevirtcorev1.InstancetypeMatcher{
				Name: "legacy-shared",
			},
			namespace: namespace,
			objects:   []ctrlruntimeclient.Object{namespaced("legacy-shared"), cluster("legacy-shared")},
			want: &kubevirtcorev1.InstancetypeMatcher{
				Name: "legacy-shared",
				Kind: "VirtualMachineInstancetype",
			},
		},
		{
			name: "legacy empty kind falls back to cluster scope",
			matcher: &kubevirtcorev1.InstancetypeMatcher{
				Name: "legacy-cluster",
			},
			namespace: namespace,
			objects:   []ctrlruntimeclient.Object{cluster("legacy-cluster")},
			want: &kubevirtcorev1.InstancetypeMatcher{
				Name: "legacy-cluster",
				Kind: "VirtualMachineClusterInstancetype",
			},
		},
		{
			name: "legacy empty kind without a namespace resolves cluster scope",
			matcher: &kubevirtcorev1.InstancetypeMatcher{
				Name: "legacy-cluster-no-namespace",
			},
			objects: []ctrlruntimeclient.Object{cluster("legacy-cluster-no-namespace")},
			want: &kubevirtcorev1.InstancetypeMatcher{
				Name: "legacy-cluster-no-namespace",
				Kind: "VirtualMachineClusterInstancetype",
			},
		},
		{
			name: "kind is canonicalized and matcher fields are preserved",
			matcher: &kubevirtcorev1.InstancetypeMatcher{
				Name:         "revisioned",
				Kind:         "virtualmachineclusterinstancetype",
				RevisionName: "revision-1",
			},
			namespace: namespace,
			objects:   []ctrlruntimeclient.Object{cluster("revisioned")},
			want: &kubevirtcorev1.InstancetypeMatcher{
				Name:         "revisioned",
				Kind:         "VirtualMachineClusterInstancetype",
				RevisionName: "revision-1",
			},
		},
		{
			name: "v1alpha1 instancetype remains supported",
			matcher: &kubevirtcorev1.InstancetypeMatcher{
				Name: "alpha",
				Kind: "VirtualMachineInstancetype",
			},
			namespace: namespace,
			objects: []ctrlruntimeclient.Object{
				&kubevirtinstancetypev1alpha1.VirtualMachineInstancetype{
					ObjectMeta: metav1.ObjectMeta{Name: "alpha", Namespace: namespace},
				},
			},
			want: &kubevirtcorev1.InstancetypeMatcher{
				Name: "alpha",
				Kind: "VirtualMachineInstancetype",
			},
		},
		{
			name: "v1alpha1 cluster instancetype remains supported",
			matcher: &kubevirtcorev1.InstancetypeMatcher{
				Name: "alpha-cluster",
				Kind: "VirtualMachineClusterInstancetype",
			},
			namespace: namespace,
			objects: []ctrlruntimeclient.Object{
				&kubevirtinstancetypev1alpha1.VirtualMachineClusterInstancetype{
					ObjectMeta: metav1.ObjectMeta{Name: "alpha-cluster"},
				},
			},
			want: &kubevirtcorev1.InstancetypeMatcher{
				Name: "alpha-cluster",
				Kind: "VirtualMachineClusterInstancetype",
			},
		},
		{
			name: "namespaced kind requires an infrastructure namespace",
			matcher: &kubevirtcorev1.InstancetypeMatcher{
				Name: "namespaced",
				Kind: "VirtualMachineInstancetype",
			},
			objects: []ctrlruntimeclient.Object{namespaced("namespaced")},
			wantErr: "without an infrastructure namespace",
		},
		{
			name: "unsupported kind is rejected",
			matcher: &kubevirtcorev1.InstancetypeMatcher{
				Name: "invalid",
				Kind: "OtherInstancetype",
			},
			namespace: namespace,
			wantErr:   `unsupported KubeVirt instancetype kind "OtherInstancetype"`,
		},
		{
			name: "missing legacy instancetype is rejected",
			matcher: &kubevirtcorev1.InstancetypeMatcher{
				Name: "missing",
			},
			namespace: namespace,
			wantErr:   `no VirtualMachineInstancetype or VirtualMachineClusterInstancetype found with name "missing"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := newInstancetypeClient(t, tt.objects...)
			originalMatcher := tt.matcher.DeepCopy()

			got, err := defaultVMInstanceType(context.Background(), client, tt.matcher, tt.namespace)
			if diff := cmp.Diff(originalMatcher, tt.matcher); diff != "" {
				t.Errorf("defaultVMInstanceType() mutated its input (-want +got):\n%s", diff)
			}
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tt.wantErr)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("expected error containing %q, got %q", tt.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("defaultVMInstanceType() returned an unexpected error: %v", err)
			}
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("defaultVMInstanceType() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestDefaultVMInstanceTypeDoesNotHideLookupErrors(t *testing.T) {
	const namespace = "tenant-a"

	client := &instancetypeGetErrorClient{
		Client: newInstancetypeClient(t, &kubevirtinstancetypev1beta1.VirtualMachineClusterInstancetype{
			ObjectMeta: metav1.ObjectMeta{Name: "shared"},
		}),
		getError: func(obj ctrlruntimeclient.Object) error {
			if _, ok := obj.(*kubevirtinstancetypev1beta1.VirtualMachineInstancetype); ok {
				return errors.New("access denied")
			}
			return nil
		},
	}

	_, err := defaultVMInstanceType(context.Background(), client, &kubevirtcorev1.InstancetypeMatcher{Name: "shared"}, namespace)
	if err == nil {
		t.Fatal("expected lookup error, got nil")
	}
	if !strings.Contains(err.Error(), "access denied") {
		t.Fatalf("expected lookup error to be retained, got %q", err)
	}
}

func TestDefaultVMInstanceTypeDoesNotHideClusterLookupErrors(t *testing.T) {
	client := &instancetypeGetErrorClient{
		Client: newInstancetypeClient(t, &kubevirtinstancetypev1alpha1.VirtualMachineClusterInstancetype{
			ObjectMeta: metav1.ObjectMeta{Name: "cluster"},
		}),
		getError: func(obj ctrlruntimeclient.Object) error {
			if _, ok := obj.(*kubevirtinstancetypev1beta1.VirtualMachineClusterInstancetype); ok {
				return errors.New("cluster access denied")
			}
			return nil
		},
	}

	_, err := defaultVMInstanceType(context.Background(), client, &kubevirtcorev1.InstancetypeMatcher{
		Name: "cluster",
		Kind: "VirtualMachineClusterInstancetype",
	}, "tenant-a")
	if err == nil {
		t.Fatal("expected cluster lookup error, got nil")
	}
	if !strings.Contains(err.Error(), "cluster access denied") {
		t.Fatalf("expected cluster lookup error to be retained, got %q", err)
	}
}

func TestDefaultVMInstanceTypeFallsBackWhenAPIVersionIsNotServed(t *testing.T) {
	const namespace = "tenant-a"

	client := &instancetypeGetErrorClient{
		Client: newInstancetypeClient(t, &kubevirtinstancetypev1alpha1.VirtualMachineInstancetype{
			ObjectMeta: metav1.ObjectMeta{Name: "alpha", Namespace: namespace},
		}),
		getError: func(obj ctrlruntimeclient.Object) error {
			if _, ok := obj.(*kubevirtinstancetypev1beta1.VirtualMachineInstancetype); ok {
				return &meta.NoKindMatchError{
					GroupKind:        schema.GroupKind{Group: "instancetype.kubevirt.io", Kind: "VirtualMachineInstancetype"},
					SearchedVersions: []string{"v1beta1"},
				}
			}
			return nil
		},
	}

	want := &kubevirtcorev1.InstancetypeMatcher{Name: "alpha", Kind: "VirtualMachineInstancetype"}
	got, err := defaultVMInstanceType(context.Background(), client, want, namespace)
	if err != nil {
		t.Fatalf("defaultVMInstanceType() returned an unexpected error: %v", err)
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("defaultVMInstanceType() mismatch (-want +got):\n%s", diff)
	}
}

func TestResolvedInstancetypeKindIsUsedByVirtualMachine(t *testing.T) {
	const instancetypeName = "shared"

	client := newInstancetypeClient(t,
		&kubevirtinstancetypev1beta1.VirtualMachineInstancetype{
			ObjectMeta: metav1.ObjectMeta{Name: instancetypeName, Namespace: testNamespace},
		},
		&kubevirtinstancetypev1beta1.VirtualMachineClusterInstancetype{
			ObjectMeta: metav1.ObjectMeta{Name: instancetypeName},
		},
	)
	resolved, err := defaultVMInstanceType(context.Background(), client, &kubevirtcorev1.InstancetypeMatcher{
		Name: instancetypeName,
		Kind: "VirtualMachineClusterInstancetype",
	}, testNamespace)
	if err != nil {
		t.Fatalf("defaultVMInstanceType() returned an unexpected error: %v", err)
	}

	p := &provider{configVarResolver: configvar.NewResolver(context.Background(), fakeclient)}
	machine := cloudprovidertesting.Creator{
		Name:               "exact-instancetype-kind",
		Namespace:          "kubevirt",
		ProviderSpecGetter: kubevirtProviderSpecConf{}.rawProviderSpec,
	}.CreateMachine(t)
	c, pc, err := p.getConfig(machine.Spec.ProviderSpec)
	if err != nil {
		t.Fatalf("provider.getConfig() error: %v", err)
	}
	c.Namespace = testNamespace
	c.Instancetype = resolved

	vm, err := p.newVirtualMachine(c, pc, machine, map[string]string{}, "udsn", userdata, fakeMachineDeploymentNameAndRevisionForMachineGetter())
	if err != nil {
		t.Fatalf("provider.newVirtualMachine() error: %v", err)
	}
	if diff := cmp.Diff(resolved, vm.Spec.Instancetype); diff != "" {
		t.Errorf("VirtualMachine instancetype mismatch (-want +got):\n%s", diff)
	}
}

func newInstancetypeClient(t *testing.T, objects ...ctrlruntimeclient.Object) ctrlruntimeclient.Client {
	t.Helper()

	scheme := runtime.NewScheme()
	if err := kubevirtinstancetypev1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add v1alpha1 instancetype types to scheme: %v", err)
	}
	if err := kubevirtinstancetypev1beta1.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add v1beta1 instancetype types to scheme: %v", err)
	}

	return fakectrlruntimeclient.NewClientBuilder().WithScheme(scheme).WithObjects(objects...).Build()
}

type instancetypeGetErrorClient struct {
	ctrlruntimeclient.Client
	getError func(ctrlruntimeclient.Object) error
}

func (c *instancetypeGetErrorClient) Get(ctx context.Context, key types.NamespacedName, obj ctrlruntimeclient.Object, opts ...ctrlruntimeclient.GetOption) error {
	if err := c.getError(obj); err != nil {
		return err
	}
	return c.Client.Get(ctx, key, obj, opts...)
}
