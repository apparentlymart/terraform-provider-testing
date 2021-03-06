// Code generated by "stringer -type=NestingMode"; DO NOT EDIT.

package tfschema

import "strconv"

func _() {
	// An "invalid array index" compiler error signifies that the constant values have changed.
	// Re-run the stringer command to generate them again.
	var x [1]struct{}
	_ = x[nestingInvalid-0]
	_ = x[NestingSingle-1]
	_ = x[NestingList-2]
	_ = x[NestingMap-3]
	_ = x[NestingSet-4]
}

const _NestingMode_name = "nestingInvalidNestingSingleNestingListNestingMapNestingSet"

var _NestingMode_index = [...]uint8{0, 14, 27, 38, 48, 58}

func (i NestingMode) String() string {
	if i < 0 || i >= NestingMode(len(_NestingMode_index)-1) {
		return "NestingMode(" + strconv.FormatInt(int64(i), 10) + ")"
	}
	return _NestingMode_name[_NestingMode_index[i]:_NestingMode_index[i+1]]
}
