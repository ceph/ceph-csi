package util

import (
	"testing"
)

func Test_getParentDirectory(t *testing.T) {
	type args struct {
		path string
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{
			name: "test_2",
			args: args{
				path: "/var/lib/kubelet/pods/9cda187a-7fb3-11ea-80b3-246e968d4b38/volumes/kubernetes.io~csi/pvc-9ae9405c-7fb3-11ea-80b3-246e968d4b38/mount",
			},
			want: "/var/lib/kubelet/pods/9cda187a-7fb3-11ea-80b3-246e968d4b38/volumes/kubernetes.io~csi/pvc-9ae9405c-7fb3-11ea-80b3-246e968d4b38",
		}, {
			name: "test_4",
			args: args{
				path: "/var/lib/kubelet/plugins/kubernetes.io/csi/pv/pvc-9ae9405c-7fb3-11ea-80b3-246e968d4b38/globalmount",
			},
			want: "/var/lib/kubelet/plugins/kubernetes.io/csi/pv/pvc-9ae9405c-7fb3-11ea-80b3-246e968d4b38",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := getParentDirectory(tt.args.path); got != tt.want {
				t.Errorf("getParentDirectory() = %v, want %v", got, tt.want)
			}
		})
	}
}
