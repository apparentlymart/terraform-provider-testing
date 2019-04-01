package tfobj

import (
	"fmt"

	"github.com/apparentlymart/terraform-sdk/tfschema"
	"github.com/zclconf/go-cty/cty"
)

// PlanReader is an extension of ObjectReader that provides access to
// information about the prior state and configuration that a plan is being
// built for.
//
// The main object represented by a PlanReader is the "planned new object" for
// a change to a managed resource instance.
type PlanReader interface {
	ObjectReader

	// Action describes the action to be taken for the object the recieving
	// PlanBuilder is representing.
	//
	// Some action types constrain the capabilities of the PlanBuilder, as follows:
	//
	// If the action is Create, the PriorReader result is always nil and AttrChange
	// will always return null as the prior value.
	//
	// If the action is Delete, the only valid actions are to call PriorReader to
	// inspect the object being deleted, to call AttrChange which will always return
	// null as the planned value, or to call ObjectVal to obtain a null object value.
	// All other methods will panic.
	//
	// If the action is Update then all PlanBuilder operations are available.
	//
	// Action Read is never used by PlanBuilder.
	Action() Action

	// PriorReader returns an ObjectReader for the prior object when planning
	// for an update operation. Returns nil when planning for a create
	// operation, because there is no prior object in that case.
	PriorReader() ObjectReader

	// AttrChange returns the value of the given attribute from the prior
	// object and the planned new object respectively. When planning for
	// a "create" operation, the prior object is always null.
	AttrChange(name string) (prior, planned cty.Value)

	// AttrHasChange returns true if the prior value for the attribute of the
	// given name is different than the planned new value for that same
	// attribute.
	AttrHasChange(name string) bool

	// The BlockPlan... family of methods echoes the Block...
	// family of methods from the ObjectReader type but they each return
	// a PlanReader for the corresponding requested block(s), rather than just
	// an ObjectReader.
	BlockPlanSingle(blockType string) PlanReader
	BlockPlanList(blockType string) []PlanReader
	BlockPlanMap(blockType string) map[string]PlanReader
	BlockPlanFromList(blockType string, idx int) PlanReader
	BlockPlanFromMap(blockType string, key string) PlanReader
}

// PlanBuilder is an extension of ObjectBuilder that provides access to
// information about the prior state and configuration that a plan is being
// built for.
//
// The object being built by a PlanBuilder is the "planned new object" for
// a change to a managed resource instance, and so as such it must reflect
// the provider's best possible prediction of what new object will result for
// the resource instance after the plan is applied. Unknown values must be
// used as placeholders for any attribute values the provider cannot predict
// during the plan phase; any known attribute values in the planned object are
// required to exactly match the final result, or Terraform Core will consider
// that a bug in the provider and abort the apply operation.
type PlanBuilder interface {
	PlanReader

	// ConfigReader returns an ObjectReader for the object representing the
	// configuration as written by the user. The config object has values
	// only for attributes that were set in the configuration; all other
	// attributes have null values, allowing the provider to determine whether
	// it is appropriate to substitute a default value for an attribute that
	// is marked as Computed.
	//
	// Although this is a read-only method, it is on PlanBuilder rather than
	// PlanReader because the configuration is consulted only during plan
	// construction. The provider should perform any analysis of the
	// configuration it needs during planning and record its decisions in
	// the planned new value for use during the apply step.
	ConfigReader() ObjectReader

	// CanProvideAttrDefault returns true if and only if the attribute of the
	// given name is marked as Computed in the schema and that attribute has
	// a null value in the user configuration. In that case, a provider is
	// permitted to provide a default value during the plan phase, which might
	// be an unknown value if the final result will not be known until the
	// apply phase.
	//
	// PlanBuilder won't prevent attempts to set defaults that violate these
	// rules, but Terraform Core itself will reject any plan that contradicts
	// explicit values given by the user in configuration.
	CanProvideAttrDefault(name string) bool

	// SetAttrUnknown is equivalent to calling SetAttr with an unknown value
	// of the appropriate type for the given attribute. It just avoids the
	// need for the caller to construct such a value.
	SetAttrUnknown(name string)

	// SetAttrNull is equivalent to calling SetAttr with a null value
	// of the appropriate type for the given attribute. It just avoids the
	// need for the caller to construct such a value.
	SetAttrNull(name string)

	// The BlockPlanBuilder... family of methods echoes the BlockBuilder...
	// family of methods from the ObjectBuilder type but they each return
	// a PlanBuilder for the corresponding requested block(s), rather than just
	// an ObjectBuilder.
	//
	// A plan is not permitted to change the collection of blocks, only to
	// provide information about the results of nested attributes that are
	// marked as Computed in the schema nad that have not been set in
	// configuration.
	BlockPlanBuilderSingle(blockType string) PlanBuilder
	BlockPlanBuilderList(blockType string) []PlanBuilder
	BlockPlanBuilderMap(blockType string) map[string]PlanBuilder
	BlockPlanBuilderFromList(blockType string, idx int) PlanBuilder
	BlockPlanBuilderFromMap(blockType string, key string) PlanBuilder

	// SetAttr is the same as for ObjectBuilder.
	SetAttr(name string, val cty.Value)

	// The Block... family of methods are the same as for ObjectBuilder.
	BlockBuilderSingle(blockType string) ObjectBuilder
	BlockBuilderList(blockType string) []ObjectBuilder
	BlockBuilderMap(blockType string) map[string]ObjectBuilder
	BlockBuilderFromList(blockType string, idx int) ObjectBuilder
	BlockBuilderFromMap(blockType string, key string) ObjectBuilder
}

// Make sure that we remember to update PlanBuilder if we add anything new to
// ObjectBuilder, since that's not represented explicitly in the decl above.
var _ ObjectBuilder = PlanBuilder(nil)

// Action represents an action to be taken.
type Action int

const (
	actionInvalid Action = iota
	Read
	Create
	Update
	Delete
)

type planBuilder struct {
	action  Action
	schema  *tfschema.BlockType
	prior   ObjectReader
	config  ObjectReader
	planned ObjectBuilder
}

// NewPlanReader constructs a PlanReader for an already-created plan, whose
// planned new object is described by "planned".
func NewPlanReader(schema *tfschema.BlockType, prior, planned cty.Value) PlanReader {
	// We just use a partially-configured PlanBuilder for this, because
	// PlanBuilder is a superset of PlanReader anyway. Technically this means
	// that a caller could type-assert this result to PlanBuilder and then
	// get some weird behavior, but that would be a very strange thing to do.
	// (If you're a provider developer reading this: please don't do it; we
	// might break this implementation detail in a future release.)
	return newPlanBuilder(schema, prior, cty.NilVal, planned)
}

// NewPlanBuilder constructs a PlanBuilder with the given prior, config, and
// proposed objects, ready to be used to customize the proposed object and
// ultimately create a planned new object to return.
func NewPlanBuilder(schema *tfschema.BlockType, prior, config, planned cty.Value) PlanBuilder {
	return newPlanBuilder(schema, prior, config, planned)
}

func newPlanBuilder(schema *tfschema.BlockType, prior, config, proposed cty.Value) PlanBuilder {
	var priorReader, configReader ObjectReader
	if prior != cty.NilVal && !prior.IsNull() {
		priorReader = NewObjectReader(schema, prior)
	}
	if config != cty.NilVal && !config.IsNull() {
		configReader = NewObjectReader(schema, config)
	}
	var plannedBuilder ObjectBuilder
	if proposed != cty.NilVal && !proposed.IsNull() {
		plannedBuilder = NewObjectBuilder(schema, proposed)
	}
	action := Update
	switch {
	case proposed.IsNull():
		action = Delete
	case prior.IsNull():
		action = Create
	}
	return &planBuilder{
		schema:  schema,
		action:  action,
		prior:   priorReader,
		config:  configReader,
		planned: plannedBuilder,
	}
}

func (b *planBuilder) Action() Action {
	return b.action
}

func (b *planBuilder) Schema() *tfschema.BlockType {
	return b.schema
}

func (b *planBuilder) ObjectVal() cty.Value {
	return b.planned.ObjectVal()
}

func (b *planBuilder) PriorReader() ObjectReader {
	return b.prior
}

func (b *planBuilder) ConfigReader() ObjectReader {
	if b.config == nil {
		panic("configuration is available only during the plan phase")
	}
	return b.config
}

func (b *planBuilder) Attr(name string) cty.Value {
	b.requireWritable()
	return b.planned.Attr(name)
}

func (b *planBuilder) SetAttr(name string, val cty.Value) {
	b.requireWritable()
	b.planned.SetAttr(name, val)
}

func (b *planBuilder) AttrChange(name string) (prior cty.Value, planned cty.Value) {
	attrS, ok := b.Schema().Attributes[name]
	if !ok {
		panic(fmt.Sprintf("%q is not an attribute", name))
	}
	if b.prior != nil {
		prior = b.prior.Attr(name)
	} else {
		prior = cty.NullVal(attrS.Type)
	}
	if b.planned != nil {
		planned = b.Attr(name)
	} else {
		planned = cty.NullVal(attrS.Type)
	}
	return
}

func (b *planBuilder) AttrHasChange(name string) bool {
	prior, planned := b.AttrChange(name)
	eqV := planned.Equals(prior)
	if !eqV.IsKnown() {
		// if unknown values are present then we will conservatively assume
		// a change is coming, though we might find out during apply that the
		// known result actually matches prior after all.
		return true
	}
	return eqV.True()
}

func (b *planBuilder) CanProvideAttrDefault(name string) bool {
	attrS, ok := b.Schema().Attributes[name]
	if !ok {
		panic(fmt.Sprintf("%q is not an attribute", name))
	}
	switch {
	case b.planned == nil:
		return false
	case !attrS.Computed:
		return false
	case b.Attr(name).IsNull():
		return true
	default:
		return false
	}
}

func (b *planBuilder) SetAttrUnknown(name string) {
	attrS, ok := b.Schema().Attributes[name]
	if !ok {
		panic(fmt.Sprintf("%q is not an attribute", name))
	}
	b.SetAttr(name, cty.UnknownVal(attrS.Type))
}

func (b *planBuilder) SetAttrNull(name string) {
	attrS, ok := b.Schema().Attributes[name]
	if !ok {
		panic(fmt.Sprintf("%q is not an attribute", name))
	}
	b.SetAttr(name, cty.NullVal(attrS.Type))
}

func (b *planBuilder) BlockCount(typeName string) int {
	return b.planned.BlockCount(typeName)
}

func (b *planBuilder) BlockSingle(typeName string) ObjectReader {
	return b.planned.BlockSingle(typeName)
}

func (b *planBuilder) BlockBuilderSingle(typeName string) ObjectBuilder {
	b.requireWritable()
	return b.planned.BlockBuilderSingle(typeName)
}

func (b *planBuilder) BlockList(typeName string) []ObjectReader {
	return b.planned.BlockList(typeName)
}

func (b *planBuilder) BlockBuilderList(typeName string) []ObjectBuilder {
	b.requireWritable()
	return b.planned.BlockBuilderList(typeName)
}

func (b *planBuilder) BlockFromList(typeName string, idx int) ObjectReader {
	return b.planned.BlockFromList(typeName, idx)
}

func (b *planBuilder) BlockBuilderFromList(typeName string, idx int) ObjectBuilder {
	b.requireWritable()
	return b.planned.BlockBuilderFromList(typeName, idx)
}

func (b *planBuilder) BlockMap(typeName string) map[string]ObjectReader {
	return b.planned.BlockMap(typeName)
}

func (b *planBuilder) BlockBuilderMap(typeName string) map[string]ObjectBuilder {
	b.requireWritable()
	return b.planned.BlockBuilderMap(typeName)
}

func (b *planBuilder) BlockFromMap(typeName string, key string) ObjectReader {
	return b.planned.BlockFromMap(typeName, key)
}

func (b *planBuilder) BlockBuilderFromMap(typeName string, key string) ObjectBuilder {
	b.requireWritable()
	return b.planned.BlockBuilderFromMap(typeName, key)
}

func (b *planBuilder) BlockPlanBuilderSingle(typeName string) PlanBuilder {
	blockS, ok := b.Schema().NestedBlockTypes[typeName]
	if !ok || blockS.Nesting != tfschema.NestingSingle {
		panic(fmt.Sprintf("%q is not a nested block type of tfschema.NestingSingle", typeName))
	}

	var priorReader, configReader ObjectReader
	var plannedBuilder ObjectBuilder

	if b.prior != nil {
		priorReader = b.prior.BlockSingle(typeName)
	}
	if b.config != nil {
		configReader = b.config.BlockSingle(typeName)
	}
	if b.planned != nil {
		plannedBuilder = b.planned.BlockBuilderSingle(typeName)
	}

	return b.subBuilder(blockS, priorReader, configReader, plannedBuilder)
}

func (b *planBuilder) BlockPlanBuilderList(typeName string) []PlanBuilder {
	var count int
	if b.planned != nil {
		count = b.planned.BlockCount(typeName)
	}
	if b.prior != nil {
		if priorCount := b.prior.BlockCount(typeName); priorCount > count {
			count = priorCount
		}
	}
	if b.config != nil {
		if configCount := b.config.BlockCount(typeName); configCount > count {
			count = configCount
		}
	}
	if count == 0 {
		return nil
	}
	ret := make([]PlanBuilder, count)
	for i := range ret {
		ret[i] = b.BlockPlanBuilderFromList(typeName, i)
	}
	return ret
}

func (b *planBuilder) BlockPlanBuilderFromList(typeName string, idx int) PlanBuilder {
	blockS, ok := b.Schema().NestedBlockTypes[typeName]
	if !ok || blockS.Nesting != tfschema.NestingList {
		panic(fmt.Sprintf("%q is not a nested block type of tfschema.NestingList", typeName))
	}

	var priorReader, configReader ObjectReader
	var plannedBuilder ObjectBuilder

	if b.prior != nil && b.prior.BlockCount(typeName) > idx {
		priorReader = b.prior.BlockFromList(typeName, idx)
	}
	if b.config != nil && b.config.BlockCount(typeName) > idx {
		configReader = b.config.BlockFromList(typeName, idx)
	}
	if b.planned != nil && b.planned.BlockCount(typeName) > idx {
		plannedBuilder = b.planned.BlockBuilderFromList(typeName, idx)
	}

	return b.subBuilder(blockS, priorReader, configReader, plannedBuilder)
}

func (b *planBuilder) BlockPlanBuilderMap(typeName string) map[string]PlanBuilder {
	blockS, ok := b.Schema().NestedBlockTypes[typeName]
	if !ok || blockS.Nesting != tfschema.NestingMap {
		panic(fmt.Sprintf("%q is not a nested block type of tfschema.NestingMap", typeName))
	}

	var priorReaders, configReaders map[string]ObjectReader
	var plannedBuilders map[string]ObjectBuilder

	if b.prior != nil {
		priorReaders = b.prior.BlockMap(typeName)
	}
	if b.config != nil {
		configReaders = b.config.BlockMap(typeName)
	}
	if b.planned != nil {
		plannedBuilders = b.planned.BlockBuilderMap(typeName)
	}

	names := make(map[string]struct{})
	for k := range priorReaders {
		names[k] = struct{}{}
	}
	for k := range configReaders {
		names[k] = struct{}{}
	}
	for k := range plannedBuilders {
		names[k] = struct{}{}
	}

	ret := make(map[string]PlanBuilder, len(names))
	for k := range names {
		ret[k] = b.subBuilder(
			blockS,
			priorReaders[k],
			configReaders[k],
			plannedBuilders[k],
		)
	}
	return ret
}

func (b *planBuilder) BlockPlanBuilderFromMap(typeName string, key string) PlanBuilder {
	blockS, ok := b.Schema().NestedBlockTypes[typeName]
	if !ok || blockS.Nesting != tfschema.NestingMap {
		panic(fmt.Sprintf("%q is not a nested block type of tfschema.NestingMap", typeName))
	}

	var priorReader, configReader ObjectReader
	var plannedBuilder ObjectBuilder

	if b.prior != nil {
		priorReader = b.prior.BlockFromMap(typeName, key)
	}
	if b.config != nil {
		configReader = b.config.BlockFromMap(typeName, key)
	}
	if b.planned != nil {
		plannedBuilder = b.planned.BlockBuilderFromMap(typeName, key)
	}

	return b.subBuilder(blockS, priorReader, configReader, plannedBuilder)
}

func (b *planBuilder) BlockPlanSingle(typeName string) PlanReader {
	ret := b.BlockPlanBuilderSingle(typeName)
	if ret == nil {
		return nil // avoid returning a typed nil
	}
	return ret
}

func (b *planBuilder) BlockPlanList(typeName string) []PlanReader {
	builders := b.BlockPlanBuilderList(typeName)
	if len(builders) == 0 {
		return nil
	}
	ret := make([]PlanReader, len(builders))
	for i, builder := range builders {
		ret[i] = builder
	}
	return ret
}

func (b *planBuilder) BlockPlanFromList(typeName string, idx int) PlanReader {
	ret := b.BlockPlanBuilderFromList(typeName, idx)
	if ret == nil {
		return nil // avoid returning a typed nil
	}
	return ret
}

func (b *planBuilder) BlockPlanFromMap(typeName string, key string) PlanReader {
	ret := b.BlockPlanBuilderFromMap(typeName, key)
	if ret == nil {
		return nil // avoid returning a typed nil
	}
	return ret
}

func (b *planBuilder) BlockPlanMap(typeName string) map[string]PlanReader {
	builders := b.BlockPlanBuilderMap(typeName)
	if len(builders) == 0 {
		return nil
	}
	ret := make(map[string]PlanReader, len(builders))
	for k, builder := range builders {
		ret[k] = builder
	}
	return ret
}

func (b *planBuilder) requireWritable() {
	if b.planned == nil {
		panic("cannot alter plan for object that will be deleted")
	}
}

func (b *planBuilder) subBuilder(schema *tfschema.NestedBlockType, prior, config ObjectReader, planned ObjectBuilder) PlanBuilder {
	action := Update
	switch {
	case planned == nil:
		action = Delete
	case prior == nil:
		action = Create
	}
	return &planBuilder{
		action:  action,
		prior:   prior,
		config:  config,
		planned: planned,
	}
}
