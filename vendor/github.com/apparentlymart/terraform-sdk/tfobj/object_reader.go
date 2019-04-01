package tfobj

import (
	"fmt"

	"github.com/apparentlymart/terraform-sdk/tfschema"
	"github.com/zclconf/go-cty/cty"
)

// An ObjectReader has methods to read data from a value that conforms to a
// particular schema, such as a resource type configuration.
type ObjectReader interface {
	// Schema returns the schema that the object conforms to. Do not modify
	// any part of the returned schema.
	Schema() *tfschema.BlockType

	// ObjectVal returns the whole object that the ObjectReader is providing
	// access to. The result has a type that conforms to the reader's schema.
	ObjectVal() cty.Value

	// Attr returns the value for the attribute of the given name. It will
	// panic if the given name is not defined as an attribute for this object
	// in its schema.
	Attr(name string) cty.Value

	// BlockCount returns the number of blocks present of the given type, or
	// panics if the given name isn't declared as a block type in the schema.
	BlockCount(blockType string) int

	// The "Block..." family of methods all interact with nested blocks.
	//
	// BlockSingle, BlockList, and BlockMap allow reading all of the blocks of
	// a particular type, with each one being appropriate for a different
	// tfschema.NestingMode. These methods will panic if the method called isn't
	// compatible with the nesting mode. (BlockList can be used with NestingSet).
	//
	// BlockFromList and BlockFromMap similarly allow extracting a single nested
	// block from a collection of blocks of a particular type using a suitable
	// key. BlockFromList can be used only with NestingList block types and
	// BlockFromMap only with NestingMap block types. Neither method can be
	// used with NestingSet block types because set elements do not have keys.
	// These methods will panic if used with an incompatible block type.
	BlockSingle(blockType string) ObjectReader
	BlockList(blockType string) []ObjectReader
	BlockMap(blockType string) map[string]ObjectReader
	BlockFromList(blockType string, idx int) ObjectReader
	BlockFromMap(blockType string, key string) ObjectReader
}

// NewObjectReader constructs a new ObjectReader for reading the given object
// value, which must be a non-null, known value whose type conforms to the
// implied type of the recieving schema, or the results are undefined.
func NewObjectReader(schema *tfschema.BlockType, obj cty.Value) ObjectReader {
	if obj.IsNull() || !obj.IsKnown() {
		panic("ObjectReader called with object that isn't known and non-null")
	}
	if !obj.Type().IsObjectType() {
		panic("ObjectReader called with non-object value")
	}
	return &objectReaderVal{
		schema: schema,
		v:      obj,
	}
}

type objectReaderVal struct {
	schema *tfschema.BlockType
	v      cty.Value
}

var _ ObjectReader = (*objectReaderVal)(nil)

func (r *objectReaderVal) Schema() *tfschema.BlockType {
	return r.schema
}

func (r *objectReaderVal) ObjectVal() cty.Value {
	return r.v
}

func (r *objectReaderVal) Attr(name string) cty.Value {
	_, exists := r.schema.Attributes[name]
	if !exists {
		panic(fmt.Sprintf("attempt to read non-attribute %q with Attr", name))
	}
	return r.v.GetAttr(name)
}

func (r *objectReaderVal) BlockCount(blockType string) int {
	blockS, obj := r.blockVal(blockType)
	switch blockS.Nesting {
	case tfschema.NestingSingle:
		if obj.IsNull() {
			return 0
		}
		return 1
	default:
		if obj.IsNull() || !obj.IsKnown() {
			// Should never happen when Terraform is behaving itself, but
			// we'll be robust to avoid a crash here.
			return 0
		}
		return obj.LengthInt()
	}
}

func (r *objectReaderVal) BlockSingle(blockType string) ObjectReader {
	blockS, obj := r.blockVal(blockType)
	if blockS.Nesting != tfschema.NestingSingle {
		panic(fmt.Sprintf("attempt to read block type %q (%s) with BlockSingle method", blockType, blockS.Nesting))
	}
	return &objectReaderVal{
		schema: &blockS.Content,
		v:      obj,
	}
}

func (r *objectReaderVal) BlockList(blockType string) []ObjectReader {
	blockS, list := r.blockVal(blockType)
	if blockS.Nesting != tfschema.NestingList && blockS.Nesting != tfschema.NestingSet {
		panic(fmt.Sprintf("attempt to read block type %q (%s) with BlockList method", blockType, blockS.Nesting))
	}
	if list.IsNull() || !list.IsKnown() {
		// Should never happen when Terraform is behaving itself, but
		// we'll be robust to avoid a crash here.
		return nil
	}
	l := list.LengthInt()
	ret := make([]ObjectReader, 0, l)
	for it := list.ElementIterator(); it.Next(); {
		_, v := it.Element()
		ret = append(ret, &objectReaderVal{
			schema: &blockS.Content,
			v:      v,
		})
	}
	return ret
}

func (r *objectReaderVal) BlockMap(blockType string) map[string]ObjectReader {
	blockS, m := r.blockVal(blockType)
	if blockS.Nesting != tfschema.NestingMap {
		panic(fmt.Sprintf("attempt to read block type %q (%s) with BlockMap method", blockType, blockS.Nesting))
	}
	if m.IsNull() || !m.IsKnown() {
		// Should never happen when Terraform is behaving itself, but
		// we'll be robust to avoid a crash here.
		return nil
	}
	l := m.LengthInt()
	ret := make(map[string]ObjectReader, l)
	for it := m.ElementIterator(); it.Next(); {
		k, v := it.Element()
		ret[k.AsString()] = &objectReaderVal{
			schema: &blockS.Content,
			v:      v,
		}
	}
	return ret
}

func (r *objectReaderVal) BlockFromList(blockType string, idx int) ObjectReader {
	blockS, list := r.blockVal(blockType)
	if blockS.Nesting != tfschema.NestingList {
		panic(fmt.Sprintf("attempt to read block type %q (%s) with BlockFromList method", blockType, blockS.Nesting))
	}
	v := list.Index(cty.NumberIntVal(int64(idx)))
	return &objectReaderVal{
		schema: &blockS.Content,
		v:      v,
	}
}

func (r *objectReaderVal) BlockFromMap(blockType string, key string) ObjectReader {
	blockS, list := r.blockVal(blockType)
	if blockS.Nesting != tfschema.NestingMap {
		panic(fmt.Sprintf("attempt to read block type %q (%s) with BlockFromMap method", blockType, blockS.Nesting))
	}
	v := list.Index(cty.StringVal(key))
	return &objectReaderVal{
		schema: &blockS.Content,
		v:      v,
	}
}

func (r *objectReaderVal) blockVal(blockType string) (*tfschema.NestedBlockType, cty.Value) {
	blockS, exists := r.schema.NestedBlockTypes[blockType]
	if !exists {
		panic(fmt.Sprintf("attempt to read non-block-type %q with block method", blockType))
	}
	return blockS, r.v.GetAttr(blockType)
}
