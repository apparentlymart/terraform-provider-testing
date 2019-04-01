package tfobj

import (
	"github.com/zclconf/go-cty/cty/gocty"
)

// Decode attempts to unpack the data from the given reader's underlying object
// using the gocty package.
func Decode(r ObjectReader, to interface{}) error {
	obj := r.ObjectVal()
	return gocty.FromCtyValue(obj, to)
}

// TODO: Also an Encode function that takes an ObjectBuilderFull and populates
// it with the result of reverse-gocty on a given interface{}.
