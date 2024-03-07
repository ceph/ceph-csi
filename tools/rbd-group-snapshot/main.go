package main

import (
	"fmt"

	"github.com/ceph/go-ceph/rados"
	"github.com/ceph/go-ceph/rbd"
)

var (
	imageNames = []string{
		"first-volume",
		"second-volume",
	}

	restoreName = "restored-image"

	pool = "ocs-storagecluster-cephblockpool"

	group     = "all-the-volumes"
	groupSnap = "all-the-snapshots"
)

type rbdGroupTest struct {
	conn  *rados.Conn
	ioctx *rados.IOContext
}

func main() {
	rgt := &rbdGroupTest{}

	rgt.connect()
	defer rgt.conn.Shutdown()

	rgt.createImages()
	defer rgt.removeImages()

	rgt.createGroup()
	defer rgt.removeGroup()

	rgt.addImagesToGroup()
	defer rgt.removeImagesFromGroup()

	rgt.createGroupSnapshot()
	defer rgt.removeGroupSnapshot()

	fmt.Println("images are still in the group")
	rgt.listSnapshots()

	rgt.removeImagesFromGroup()

	fmt.Println("images have been removed from the group")
	rgt.listSnapshots()

	rgt.removeGroup() // fails as there is still a group snapshot?

	fmt.Println("the group has been removed - expected to fail")
	rgt.listSnapshots()

	fmt.Println("the group snapshot has been removed")
	rgt.removeGroupSnapshot()

	rgt.listSnapshots()

//	rgt.restoreFromSnapshot()
//	defer rgt.removeRestoredImage()
}

func (rgt *rbdGroupTest) connect() {
	conn, err := rados.NewConn()
	if err != nil {
		panic(err)
	}

	err = conn.ReadDefaultConfigFile()
	if err != nil {
		panic(err)
	}

	err = conn.Connect()
	if err != nil {
		panic(err)
	}

	rgt.conn = conn

	ioctx, err := conn.OpenIOContext(pool)
	if err != nil {
		panic(err)
	}

	rgt.ioctx = ioctx
}

func (rgt *rbdGroupTest) createImages() {
	for _, name := range imageNames {
		_, err := rbd.Create(rgt.ioctx, name, uint64(1<<22), 22)
		if err != nil {
			panic(err)
		}
	}
}

func (rgt *rbdGroupTest) removeImages() {
	fmt.Println("removing the images")

	for _, name := range imageNames {
		err := rbd.RemoveImage(rgt.ioctx, name)
		if err != nil {
			fmt.Printf("failed to remove image %q: %v\n", name, err)
		}
	}
}

func (rgt *rbdGroupTest) createGroup() {
	err := rbd.GroupCreate(rgt.ioctx, group)
	if err != nil {
		panic(err)
	}
}

func (rgt *rbdGroupTest) removeGroup() {
	fmt.Println("removing the group")

	err := rbd.GroupRemove(rgt.ioctx, group)
	if err != nil {
		fmt.Printf("failed to remove group %q: %v\n", group, err)
	}
}

func (rgt *rbdGroupTest) addImagesToGroup() {
	for _, name := range imageNames {
		err := rbd.GroupImageAdd(rgt.ioctx, group, rgt.ioctx, name)
		if err != nil {
			panic(err)
		}
	}
}

func (rgt *rbdGroupTest) removeImagesFromGroup() {
	fmt.Println("removing images from the group")

	for _, name := range imageNames {
		err := rbd.GroupImageRemove(rgt.ioctx, group, rgt.ioctx, name)
		if err != nil {
			fmt.Printf("failed to remove image %q from group %q: %v\n", name, group, err)
		}
	}
}

func (rgt *rbdGroupTest) createGroupSnapshot() {
	err := rbd.GroupSnapCreate(rgt.ioctx, group, groupSnap)
	if err != nil {
		panic(err)
	}
}

func (rgt *rbdGroupTest) removeGroupSnapshot() {
	fmt.Println("removing the group snapshot")

	err := rbd.GroupSnapRemove(rgt.ioctx, group, groupSnap)
	if err != nil {
		fmt.Printf("failed to remove group snapshot %q: %v\n", groupSnap, err)
	}
}

func (rgt *rbdGroupTest) listSnapshots() {
	img, err := rbd.OpenImage(rgt.ioctx, imageNames[0], rbd.NoSnapshot)
	if err != nil {
		panic(err)
	}
	defer img.Close()

	snaps, err := img.GetSnapshotNames()
	if err != nil {
		panic(err)
	}

	fmt.Printf("listing %d snapshots for image %q\n", len(snaps), imageNames[0])
	for _, snap := range snaps {
		fmt.Printf("Snapshot: %+v\n", snap)
	}
}

func (rgt *rbdGroupTest) restoreFromSnapshot() {
	img, err := rbd.OpenImage(rgt.ioctx, imageNames[0], rbd.NoSnapshot)
	if err != nil {
		panic(err)
	}
	defer img.Close()

	snaps, err := img.GetSnapshotNames()
	if err != nil {
		panic(err)
	}

	options := rbd.NewRbdImageOptions()
	defer options.Destroy()
	err = options.SetUint64(rbd.ImageOptionOrder, 22)
	if err != nil {
		panic(err)
	}
//	err = options.SetUint64(rbd.ImageOptionFeatures, 1)
//	if err != nil {
//		panic(err)
//	}

/*
	fmt.Printf("restoring image %q from parent %q at snapshot %q\n", restoreName, imageNames[0], snaps[0].Name)
	snap := img.GetSnapshot(snaps[0].Name)
	err = snap.Protect()
	if err != nil {
		panic(err)
	}
	defer snap.Unprotect()

	err = rbd.CloneFromImage(img, snaps[0].Name, rgt.ioctx, restoreName, options)
	if err != nil {
		panic(err)
	}
*/

/*
	// alternative to the above -- segfaults, needs a snapshot
	fmt.Printf("restoring image %q from parent %q without a snapshot\n", restoreName, imageNames[0])
	err = rbd.CloneFromImage(img, rbd.NoSnapshot, rgt.ioctx, restoreName, options)
	if err != nil {
		panic(err)
	}
	defer rbd.RemoveImage(rgt.ioctx, restoreName)

	restored, err := rbd.OpenImage(rgt.ioctx, restoreName, rbd.NoSnapshot)
	if err != nil {
		panic(err)
	}
	defer restored.Close()

	//err = restored.SetSnapshot(snaps[0].Name)
	err = restored.SetSnapByID(snaps[0].Id)
	if err != nil {
		panic(err)
	}
*/

	// alternative to the above
	snapname := "tmp-snap"
	snap, err := img.CreateSnapshot(snapname)
	if err != nil {
		panic(err)
	}
	defer snap.Remove()

	err = snap.Protect()
	if err != nil {
		panic(err)
	}
	defer snap.Unprotect()

	fmt.Printf("restoring image %q from parent %q at snapshot %q\n", restoreName, imageNames[0], snapname)
	err = rbd.CloneFromImage(img, snapname, rgt.ioctx, restoreName, options)
	if err != nil {
		panic(err)
	}
	defer rbd.RemoveImage(rgt.ioctx, restoreName)

	restored, err := rbd.OpenImage(rgt.ioctx, restoreName, rbd.NoSnapshot)
	if err != nil {
		panic(err)
	}
	defer restored.Close()

	err = restored.SetSnapByID(snaps[0].Id)
	if err != nil {
		panic(err)
	}
}

func (rgt *rbdGroupTest) removeRestoredImage() {
	fmt.Println("removing the restored image")

	err := rbd.RemoveImage(rgt.ioctx, restoreName)
	if err != nil {
		fmt.Printf("failed to remove image %q: %v\n", restoreName, err)
	}
}
