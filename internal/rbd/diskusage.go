/*
Copyright 2021 The Ceph-CSI Authors.

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

package rbd

import (
	"fmt"
)

// Sparsify checks the size of the objects in the RBD image and calls
// rbd_sparify() to free zero-filled blocks and reduce the storage consumption
// of the image.
func (ri *rbdImage) Sparsify() error {
	image, err := ri.open()
	if err != nil {
		return err
	}
	defer image.Close()

	imageInfo, err := image.Stat()
	if err != nil {
		return err
	}

	err = image.Sparsify(1 << imageInfo.Order)
	if err != nil {
		return fmt.Errorf("failed to sparsify image: %w", err)
	}

	return nil
}
