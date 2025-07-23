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
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
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
