package internal

import (
	"reflect"
	"testing"
)

func TestMountedTargetsUnderFromMountinfo(t *testing.T) {
	root := "/var/lib/dockan/images/app_latest.dockan/rootfs"
	mountinfo := `25 20 0:22 / / rw shared:1 - btrfs /dev/root rw
36 25 0:22 /volumes/data /var/lib/dockan/images/app_latest.dockan/rootfs/app/storage rw - btrfs /dev/root rw
37 25 0:22 /volumes/cache /var/lib/dockan/images/app_latest.dockan/rootfs/app/storage/cache rw - btrfs /dev/root rw
38 25 0:22 /other /var/lib/dockan/images/other_latest.dockan/rootfs/app/storage rw - btrfs /dev/root rw
39 25 0:22 /with\040space /var/lib/dockan/images/app_latest.dockan/rootfs/app/with\040space rw - btrfs /dev/root rw
`

	got := mountedTargetsUnderFromMountinfo(root, mountinfo)
	want := []string{
		"/var/lib/dockan/images/app_latest.dockan/rootfs/app/storage",
		"/var/lib/dockan/images/app_latest.dockan/rootfs/app/with space",
		"/var/lib/dockan/images/app_latest.dockan/rootfs/app/storage/cache",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("mounted targets = %#v, want %#v", got, want)
	}
}
