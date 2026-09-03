package biz

import (
	"entgo.io/contrib/entgql"
	entcore "entgo.io/ent"
)

// RestoreZeroCursorValue works around entgql encoding cursors with msgpack
// `omitempty`: when the order value is the zero of its type (for example
// ordering_weight 0) it is dropped from the cursor and decodes as nil, and
// entgql.CursorsPredicate then falls back to comparing by id only, so the next
// page repeats or skips rows. Only zero values are dropped, so the missing value
// can be rebuilt from an empty node.
//
// Callers must skip the default (ID) order: its value is the id itself and the
// nil cursor value is the intended encoding there.
func RestoreZeroCursorValue[N any](cursor *entgql.Cursor[int], value func(*N) (entcore.Value, error)) {
	if cursor == nil || cursor.Value != nil || value == nil {
		return
	}

	var zero N

	v, err := value(&zero)
	if err != nil {
		return
	}

	cursor.Value = v
}
