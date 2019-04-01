package tfobj

import (
	"fmt"

	"github.com/apparentlymart/terraform-sdk/internal/sdkdiags"
	"github.com/apparentlymart/terraform-sdk/tfschema"
	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/convert"
)

// An ObjectBuilder is a helper for gradually constructing a new value that
// conforms to a particular schema through mutation.
//
// Terraform type system values are normally immutable, but ObjectBuilder
// provides a mutable representation of an object value that can, once ready,
// be frozen into an immutable object value.
type ObjectBuilder interface {
	// ObjectBuilder extends ObjectReader, providing access to the current
	// state of the object under construction.
	//
	// Call ObjectVal for a cty.Value representation of the whole object, once
	// all mutations are complete.
	ObjectReader

	// SetAttr replaces the value of the specified attribute with the given
	// value. It will panic if the given name is not defined as an attribute
	// for this object or if the given value is not compatible with the
	// type constraint given for the attribute in the schema.
	SetAttr(name string, val cty.Value)

	// The Block... family of methods echoes the methods with similar names on
	// ObjectReader but each returns an ObjectBuilder that can be used to
	// mutate the content of the requested block.
	//
	// ObjectBuilder does not permit modifying the collection of nested blocks
	// itself, because most Terraform operations require the result to contain
	// exactly the same blocks as given in configuration.
	BlockBuilderSingle(blockType string) ObjectBuilder
	BlockBuilderList(blockType string) []ObjectBuilder
	BlockBuilderMap(blockType string) map[string]ObjectBuilder
	BlockBuilderFromList(blockType string, idx int) ObjectBuilder
	BlockBuilderFromMap(blockType string, key string) ObjectBuilder
}

// NewObjectBuilder creates and returns a new ObjectBuilder with the receiving
// schema, whose initial value is a copy of the given object value.
//
// The given value must be an object type conforming to the schema, or
// this function may panic or have other undefined behavior. To start with
// a value that has all attributes null and no nested blocks, pass cty.NilVal
// as the initial value.
func NewObjectBuilder(schema *tfschema.BlockType, initial cty.Value) ObjectBuilder {
	return newObjectBuilder(schema, initial)
}

// DeriveNewObject constructs an ObjectBuilderFull with the same schema as the
// given ObjectReader and an initial object value equal to that of the reader.
//
// This is useful when a new value is mostly equal to an existing value but
// needs a few surgical changes made in-place.
func DeriveNewObject(r ObjectReader) ObjectBuilderFull {
	return objectBuilderFull{newObjectBuilder(r.Schema(), r.ObjectVal())}
}

// ObjectBuilderFull is an extension of ObjectBuilder that additionally allows
// totally replacing the collection of nested blocks of a given type.
//
// This interface is separate because most Terraform operations do not permit
// this change. For resource types, it is allowed only for the ReadFn
// implementation in order to synchronize the collection of nested blocks with
// the collection of corresponding objects in the remote system.
type ObjectBuilderFull interface {
	ObjectBuilder

	// NewBlockBuilder returns an ObjectBuilderFull that can construct an object
	// of a type suitable to build a new nested block of the given type. It will
	// panic if no nested block type of the given name is defined.
	//
	// The returned builder is disconnected from the object that creates it
	// in the sense that modifications won't be reflected anywhere in the
	// creator. To make use of the result, call ObjectVal to obtain an
	// object value and pass it to one of the "ReplaceBlock..." methods.
	NewBlockBuilder(blockType string) ObjectBuilderFull

	// The ReplaceBlock... family of methods remove all blocks of the given
	// type and then construct new blocks from the given object(s) in their
	// place. The given nested builders must have been originally returned
	// from NewBlockBuilder on the same builder or these methods will panic.
	// These will panic also if the method used doesn't correspond with the
	// nesting mode of the given nested block type.
	ReplaceBlockSingle(blockType string, nb ObjectBuilderFull)
	ReplaceBlocksList(blockType string, nbs []ObjectBuilderFull)
	ReplaceBlocksMap(blockType string, nbs map[string]ObjectBuilderFull)
}

// NewObjectBuilderFull is like NewObjectBuilder except that it constructs an
// ObjectBuilderFull instead of just an ObjectBuilder.
func NewObjectBuilderFull(schema *tfschema.BlockType, initial cty.Value) ObjectBuilderFull {
	ob := newObjectBuilder(schema, initial)
	return objectBuilderFull{ob}
}

type objectBuilder struct {
	schema       *tfschema.BlockType
	attrs        map[string]cty.Value
	singleBlocks map[string]*objectBuilder
	listBlocks   map[string][]*objectBuilder
	mapBlocks    map[string]map[string]*objectBuilder
}

func newObjectBuilder(schema *tfschema.BlockType, initial cty.Value) *objectBuilder {
	ret := &objectBuilder{
		schema:       schema,
		attrs:        make(map[string]cty.Value),
		singleBlocks: make(map[string]*objectBuilder),
		listBlocks:   make(map[string][]*objectBuilder),
		mapBlocks:    make(map[string]map[string]*objectBuilder),
	}

	for name, attrS := range schema.Attributes {
		if initial == cty.NilVal {
			ret.attrs[name] = cty.NullVal(attrS.Type)
			continue
		}
		ret.attrs[name] = initial.GetAttr(name)
	}

	for name, blockS := range schema.NestedBlockTypes {
		switch blockS.Nesting {
		case tfschema.NestingSingle:
			if initial == cty.NilVal {
				ret.singleBlocks[name] = nil
				continue
			}
			nv := initial.GetAttr(name)
			if nv.IsNull() {
				ret.singleBlocks[name] = nil
				continue
			}
			ret.singleBlocks[name] = newObjectBuilder(&blockS.Content, nv)
		case tfschema.NestingList, tfschema.NestingSet:
			if initial == cty.NilVal {
				ret.listBlocks[name] = make([]*objectBuilder, 0)
				continue
			}
			nv := initial.GetAttr(name)
			if nv.IsKnown() && !nv.IsNull() {
				ret.listBlocks[name] = make([]*objectBuilder, 0, nv.LengthInt())
				for it := nv.ElementIterator(); it.Next(); {
					_, ev := it.Element()
					ret.listBlocks[name] = append(
						ret.listBlocks[name],
						newObjectBuilder(&blockS.Content, ev),
					)
				}
			}
		case tfschema.NestingMap:
			if initial == cty.NilVal {
				ret.mapBlocks[name] = make(map[string]*objectBuilder)
				continue
			}
			nv := initial.GetAttr(name)
			if nv.IsKnown() && !nv.IsNull() {
				ret.mapBlocks[name] = make(map[string]*objectBuilder, nv.LengthInt())
				for it := nv.ElementIterator(); it.Next(); {
					ek, ev := it.Element()
					ret.mapBlocks[name][ek.AsString()] = newObjectBuilder(&blockS.Content, ev)
				}
			}
		default:
			panic(fmt.Sprintf("unknown block type nesting mode %s for %q", blockS.Nesting, name))
		}
	}

	return ret
}

func (b *objectBuilder) Schema() *tfschema.BlockType {
	return b.schema
}

func (b *objectBuilder) ObjectVal() cty.Value {
	vals := make(map[string]cty.Value, len(b.attrs)+len(b.singleBlocks)+len(b.listBlocks)+len(b.mapBlocks))
	for name, val := range b.attrs {
		vals[name] = val
	}
	for name, nb := range b.singleBlocks {
		vals[name] = nb.ObjectVal()
	}
	for name, nbs := range b.listBlocks {
		blockS := b.schema.NestedBlockTypes[name]
		wantEty := blockS.Content.ImpliedCtyType()
		if len(nbs) == 0 {
			switch blockS.Nesting {
			case tfschema.NestingList:
				if wantEty.HasDynamicTypes() {
					vals[name] = cty.EmptyTupleVal
				} else {
					vals[name] = cty.ListValEmpty(wantEty)
				}
			case tfschema.NestingSet:
				vals[name] = cty.SetValEmpty(wantEty)
			}
			continue
		}
		subVals := make([]cty.Value, len(nbs))
		for i, nb := range nbs {
			subVals[i] = nb.ObjectVal()
		}
		switch blockS.Nesting {
		case tfschema.NestingList:
			if wantEty.HasDynamicTypes() {
				vals[name] = cty.TupleVal(subVals)
			} else {
				vals[name] = cty.ListVal(subVals)
			}
		case tfschema.NestingSet:
			vals[name] = cty.SetVal(subVals)
		}
	}
	for name, nbs := range b.mapBlocks {
		blockS := b.schema.NestedBlockTypes[name]
		wantEty := blockS.Content.ImpliedCtyType()
		if len(nbs) == 0 {
			if wantEty.HasDynamicTypes() {
				vals[name] = cty.EmptyObjectVal
			} else {
				vals[name] = cty.MapValEmpty(wantEty)
			}
			continue
		}
		subVals := make(map[string]cty.Value, len(nbs))
		for k, nb := range nbs {
			subVals[k] = nb.ObjectVal()
		}
		if wantEty.HasDynamicTypes() {
			vals[name] = cty.ObjectVal(subVals)
		} else {
			vals[name] = cty.MapVal(subVals)
		}
	}
	return cty.ObjectVal(vals)
}

func (b *objectBuilder) Attr(name string) cty.Value {
	if _, ok := b.schema.Attributes[name]; !ok {
		panic(fmt.Sprintf("no attribute named %q", name))
	}
	return b.attrs[name]
}

func (b *objectBuilder) SetAttr(name string, val cty.Value) {
	attrS, ok := b.schema.Attributes[name]
	if !ok {
		panic(fmt.Sprintf("no attribute named %q", name))
	}
	val, err := convert.Convert(val, attrS.Type)
	if err != nil {
		panic(fmt.Sprintf("unsuitable value for %q: %s", name, sdkdiags.FormatError(err)))
	}
	b.attrs[name] = val
}

func (b *objectBuilder) BlockCount(typeName string) int {
	blockS, ok := b.schema.NestedBlockTypes[typeName]
	if !ok {
		panic(fmt.Sprintf("no block type named %q", typeName))
	}
	switch blockS.Nesting {
	case tfschema.NestingSingle:
		if b.singleBlocks[typeName] == nil {
			return 0
		}
		return 1
	case tfschema.NestingList, tfschema.NestingSet:
		return len(b.listBlocks[typeName])
	case tfschema.NestingMap:
		return len(b.mapBlocks[typeName])
	default:
		panic(fmt.Sprintf("unknown block type nesting mode %s for %q", blockS.Nesting, typeName))
	}
}

func (b *objectBuilder) BlockSingle(typeName string) ObjectReader {
	ret := b.BlockBuilderSingle(typeName)
	if ret == nil {
		return nil // avoid returning typed nil
	}
	return ret
}

func (b *objectBuilder) BlockList(typeName string) []ObjectReader {
	bbs := b.BlockBuilderList(typeName)
	if len(bbs) == 0 {
		return nil
	}
	ret := make([]ObjectReader, len(bbs))
	for i, bb := range bbs {
		ret[i] = bb
	}
	return ret
}

func (b *objectBuilder) BlockFromList(typeName string, idx int) ObjectReader {
	ret := b.BlockBuilderFromList(typeName, idx)
	if ret == nil {
		return nil // avoid returning typed nil
	}
	return ret
}

func (b *objectBuilder) BlockMap(typeName string) map[string]ObjectReader {
	bbs := b.BlockBuilderMap(typeName)
	if len(bbs) == 0 {
		return nil
	}
	ret := make(map[string]ObjectReader, len(bbs))
	for k, bb := range bbs {
		ret[k] = bb
	}
	return ret
}

func (b *objectBuilder) BlockFromMap(typeName string, key string) ObjectReader {
	ret := b.BlockBuilderFromMap(typeName, key)
	if ret == nil {
		return nil // avoid returning typed nil
	}
	return ret
}

func (b *objectBuilder) BlockBuilderSingle(typeName string) ObjectBuilder {
	if blockS, ok := b.schema.NestedBlockTypes[typeName]; !ok || blockS.Nesting != tfschema.NestingSingle {
		panic(fmt.Sprintf("%q is not a nested block type of tfschema.NestingSingle", typeName))
	}
	ret := b.singleBlocks[typeName]
	if ret == nil {
		return nil // avoid returning typed nil
	}
	return ret
}

func (b *objectBuilder) BlockBuilderList(typeName string) []ObjectBuilder {
	if blockS, ok := b.schema.NestedBlockTypes[typeName]; !ok || (blockS.Nesting != tfschema.NestingList && blockS.Nesting != tfschema.NestingSet) {
		panic(fmt.Sprintf("%q is not a nested block type of tfschema.NestingList or tfschema.NestingSet", typeName))
	}
	nbs := b.listBlocks[typeName]
	if len(nbs) == 0 {
		return nil
	}
	ret := make([]ObjectBuilder, len(nbs))
	for i, nb := range nbs {
		ret[i] = nb
	}
	return ret
}

func (b *objectBuilder) BlockBuilderFromList(typeName string, idx int) ObjectBuilder {
	if blockS, ok := b.schema.NestedBlockTypes[typeName]; !ok || blockS.Nesting != tfschema.NestingList {
		panic(fmt.Sprintf("%q is not a nested block type of tfschema.NestingList", typeName))
	}
	ret := b.listBlocks[typeName][idx]
	if ret == nil {
		return nil // avoid returning typed nil
	}
	return ret
}

func (b *objectBuilder) BlockBuilderMap(typeName string) map[string]ObjectBuilder {
	if blockS, ok := b.schema.NestedBlockTypes[typeName]; !ok || blockS.Nesting != tfschema.NestingMap {
		panic(fmt.Sprintf("%q is not a nested block type of tfschema.NestingMap", typeName))
	}
	nbs := b.mapBlocks[typeName]
	if len(nbs) == 0 {
		return nil
	}
	ret := make(map[string]ObjectBuilder, len(nbs))
	for k, nb := range nbs {
		ret[k] = nb
	}
	return ret
}

func (b *objectBuilder) BlockBuilderFromMap(typeName string, key string) ObjectBuilder {
	if blockS, ok := b.schema.NestedBlockTypes[typeName]; !ok || blockS.Nesting != tfschema.NestingMap {
		panic(fmt.Sprintf("%q is not a nested block type of tfschema.NestingMap", typeName))
	}
	ret := b.mapBlocks[typeName][key]
	if ret == nil {
		return nil // avoid returning typed nil
	}
	return ret
}

type objectBuilderFull struct {
	*objectBuilder
}

func (b objectBuilderFull) NewBlockBuilder(typeName string) ObjectBuilderFull {
	blockS, ok := b.schema.NestedBlockTypes[typeName]
	if !ok {
		panic(fmt.Sprintf("%q is not a nested block type", typeName))
	}

	nb := newObjectBuilder(&blockS.Content, cty.NilVal)
	return objectBuilderFull{nb}
}

func (b objectBuilderFull) ReplaceBlockSingle(typeName string, nb ObjectBuilderFull) {
	blockS, ok := b.schema.NestedBlockTypes[typeName]
	if !ok || blockS.Nesting != tfschema.NestingSingle {
		panic(fmt.Sprintf("%q is not a nested block type of tfschema.NestingSingle", typeName))
	}
	if nb == nil {
		b.objectBuilder.singleBlocks[typeName] = nil
		return
	}
	b.objectBuilder.singleBlocks[typeName] = nb.(objectBuilderFull).objectBuilder
}

func (b objectBuilderFull) ReplaceBlocksList(typeName string, nbs []ObjectBuilderFull) {
	blockS, ok := b.schema.NestedBlockTypes[typeName]
	if !ok || (blockS.Nesting != tfschema.NestingList && blockS.Nesting != tfschema.NestingSet) {
		panic(fmt.Sprintf("%q is not a nested block type of tfschema.NestingList or tfschema.NestingSet", typeName))
	}
	if len(nbs) == 0 {
		b.objectBuilder.listBlocks[typeName] = make([]*objectBuilder, 0)
		return
	}
	new := make([]*objectBuilder, len(nbs))
	for i, nb := range nbs {
		new[i] = nb.(objectBuilderFull).objectBuilder
	}
	b.objectBuilder.listBlocks[typeName] = new
}

func (b objectBuilderFull) ReplaceBlocksMap(typeName string, nbs map[string]ObjectBuilderFull) {
	blockS, ok := b.schema.NestedBlockTypes[typeName]
	if !ok || blockS.Nesting != tfschema.NestingMap {
		panic(fmt.Sprintf("%q is not a nested block type of tfschema.NestingMap", typeName))
	}
	if len(nbs) == 0 {
		b.objectBuilder.listBlocks[typeName] = make([]*objectBuilder, 0)
		return
	}
	new := make(map[string]*objectBuilder, len(nbs))
	for k, nb := range nbs {
		new[k] = nb.(objectBuilderFull).objectBuilder
	}
	b.objectBuilder.mapBlocks[typeName] = new
}
