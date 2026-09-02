/*
Copyright 2025 The Ceph-CSI Authors.

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
package persistentvolume

import (
	"context"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func Test_shouldReconcileBasedOnDriver(t *testing.T) {
	t.Parallel()
	rbdDriver := "rbd.csi.ceph.com"
	type args struct {
		obj        client.Object
		driverName string
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{
			name: "Nil object returns false",
			args: args{
				obj:        nil,
				driverName: rbdDriver,
			},
			want: false,
		},
		{
			name: "Object under deletion returns false",
			args: args{
				obj: &corev1.PersistentVolume{
					ObjectMeta: metav1.ObjectMeta{
						DeletionTimestamp: &metav1.Time{Time: time.Now()},
					},
				},
				driverName: rbdDriver,
			},
			want: false,
		},
		{
			name: "Missing annotation returns true",
			args: args{
				obj: &corev1.PersistentVolume{
					ObjectMeta: metav1.ObjectMeta{
						Annotations: map[string]string{},
					},
				},
				driverName: rbdDriver,
			},
			want: true,
		},
		{
			name: "Annotation matches driver returns true",
			args: args{
				obj: &corev1.PersistentVolume{
					ObjectMeta: metav1.ObjectMeta{
						Annotations: map[string]string{
							"pv.kubernetes.io/provisioned-by": rbdDriver,
						},
					},
				},
				driverName: rbdDriver,
			},
			want: true,
		},
		{
			name: "Annotation does not match driver returns false",
			args: args{
				obj: &corev1.PersistentVolume{
					ObjectMeta: metav1.ObjectMeta{
						Annotations: map[string]string{
							"pv.kubernetes.io/provisioned-by": "cephfs.csi.ceph.com",
						},
					},
				},
				driverName: rbdDriver,
			},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := shouldReconcileBasedOnDriver(tt.args.obj, tt.args.driverName); got != tt.want {
				t.Errorf("shouldReconcileBasedOnDriver() = %v, want %v", got, tt.want)
			}
		})
	}
}

func vacName(name string) *string {
	return &name
}

func Test_getVolumeAttributesClassParameters(t *testing.T) {
	t.Parallel()

	scheme := runtime.NewScheme()
	if err := clientgoscheme.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add scheme: %v", err)
	}

	existingVAC := &storagev1.VolumeAttributesClass{
		ObjectMeta: metav1.ObjectMeta{Name: "qos-gold"},
		DriverName: "rbd.csi.ceph.com",
		Parameters: map[string]string{
			"maxReadIops":  "1000",
			"maxWriteIops": "2000",
		},
	}

	tests := []struct {
		name    string
		pv      *corev1.PersistentVolume
		want    map[string]string
		wantErr bool
	}{
		{
			name: "PV without VolumeAttributesClassName returns nil",
			pv: &corev1.PersistentVolume{
				Spec: corev1.PersistentVolumeSpec{VolumeAttributesClassName: nil},
			},
			want:    nil,
			wantErr: false,
		},
		{
			name: "PV with empty VolumeAttributesClassName returns nil",
			pv: &corev1.PersistentVolume{
				Spec: corev1.PersistentVolumeSpec{VolumeAttributesClassName: vacName("")},
			},
			want:    nil,
			wantErr: false,
		},
		{
			name: "PV referencing existing VolumeAttributesClass returns its parameters",
			pv: &corev1.PersistentVolume{
				Spec: corev1.PersistentVolumeSpec{
					VolumeAttributesClassName: vacName("qos-gold"),
					PersistentVolumeSource: corev1.PersistentVolumeSource{
						CSI: &corev1.CSIPersistentVolumeSource{Driver: "rbd.csi.ceph.com"},
					},
				},
			},
			want: map[string]string{
				"maxReadIops":  "1000",
				"maxWriteIops": "2000",
			},
			wantErr: false,
		},
		{
			name: "PV referencing missing VolumeAttributesClass returns error",
			pv: &corev1.PersistentVolume{
				Spec: corev1.PersistentVolumeSpec{VolumeAttributesClassName: vacName("does-not-exist")},
			},
			want:    nil,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cl := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(existingVAC.DeepCopy()).
				Build()
			r := &ReconcilePersistentVolume{client: cl}

			got, err := r.getVolumeAttributesClassParameters(context.TODO(), tt.pv)
			if (err != nil) != tt.wantErr {
				t.Fatalf("getVolumeAttributesClassParameters() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if len(got) != len(tt.want) {
				t.Fatalf("getVolumeAttributesClassParameters() = %v, want %v", got, tt.want)
			}
			for k, v := range tt.want {
				if got[k] != v {
					t.Errorf("getVolumeAttributesClassParameters()[%s] = %s, want %s", k, got[k], v)
				}
			}
		})
	}
}
