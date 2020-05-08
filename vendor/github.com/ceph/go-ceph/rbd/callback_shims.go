package rbd

/*

#include <rbd/librbd.h>

extern int diffIterateCallback(uint64_t ofs, size_t len, int exists, int index);

int callDiffIterateCallback(uint64_t ofs, size_t len, int exists, int index) {
	return diffIterateCallback(ofs, len, exists, index);
}
*/
import "C"
