// Copyright 2022 The Cockroach Authors.
//
// Use of this software is governed by the Business Source License
// included in the file licenses/BSL.txt.
//
// As of the Change Date specified in that file, in accordance with
// the Business Source License, use of this software will be governed
// by the Apache License, Version 2.0, included in the file
// licenses/APL.txt.

package descs

import (
	"context"
	"fmt"

	"github.com/cockroachdb/cockroach/pkg/kv"
	"github.com/cockroachdb/cockroach/pkg/sql/catalog"
	"github.com/cockroachdb/cockroach/pkg/sql/catalog/dbdesc"
	"github.com/cockroachdb/cockroach/pkg/sql/catalog/descpb"
	"github.com/cockroachdb/cockroach/pkg/sql/catalog/funcdesc"
	"github.com/cockroachdb/cockroach/pkg/sql/catalog/schemadesc"
	"github.com/cockroachdb/cockroach/pkg/sql/catalog/tabledesc"
	"github.com/cockroachdb/cockroach/pkg/sql/catalog/typedesc"
	"github.com/cockroachdb/cockroach/pkg/sql/pgwire/pgcode"
	"github.com/cockroachdb/cockroach/pkg/sql/pgwire/pgerror"
	"github.com/cockroachdb/cockroach/pkg/sql/sem/tree"
	"github.com/cockroachdb/cockroach/pkg/sql/sqlerrors"
	"github.com/cockroachdb/errors"
)

// ByIDGetter looks up immutable descriptors by ID.
type ByIDGetter getterBase

// Descs looks up immutable descriptors by ID.
func (g ByIDGetter) Descs(ctx context.Context, ids []descpb.ID) ([]catalog.Descriptor, error) {
	ret := make([]catalog.Descriptor, len(ids))
	if err := getDescriptorsByID(ctx, g.Descriptors(), g.KV(), g.flags, ret, ids...); err != nil {
		return nil, err
	}
	return ret, nil
}

// Desc looks up an immutable descriptor by ID.
func (g ByIDGetter) Desc(ctx context.Context, id descpb.ID) (catalog.Descriptor, error) {
	var arr [1]catalog.Descriptor
	if err := getDescriptorsByID(ctx, g.Descriptors(), g.KV(), g.flags, arr[:], id); err != nil {
		return nil, err
	}
	return arr[0], nil
}

// Database looks up an immutable database descriptor by ID.
func (g ByIDGetter) Database(
	ctx context.Context, id descpb.ID,
) (catalog.DatabaseDescriptor, error) {
	desc, err := g.Desc(ctx, id)
	if err != nil {
		if errors.Is(err, catalog.ErrDescriptorNotFound) {
			if g.flags.isOptional {
				return nil, nil
			}
			return nil, sqlerrors.NewUndefinedDatabaseError(fmt.Sprintf("[%d]", id))
		}
		return nil, err
	}
	db, ok := desc.(catalog.DatabaseDescriptor)
	if !ok {
		return nil, sqlerrors.NewUndefinedDatabaseError(fmt.Sprintf("[%d]", id))
	}
	return db, nil
}

// Schema looks up an immutable schema descriptor by ID.
func (g ByIDGetter) Schema(ctx context.Context, id descpb.ID) (catalog.SchemaDescriptor, error) {
	desc, err := g.Desc(ctx, id)
	if err != nil {
		if errors.Is(err, catalog.ErrDescriptorNotFound) {
			if g.flags.isOptional {
				return nil, nil
			}
			return nil, sqlerrors.NewUndefinedSchemaError(fmt.Sprintf("[%d]", id))
		}
		return nil, err
	}
	sc, ok := desc.(catalog.SchemaDescriptor)
	if !ok {
		return nil, sqlerrors.NewUndefinedSchemaError(fmt.Sprintf("[%d]", id))
	}
	return sc, nil
}

// Table looks up an immutable table descriptor by ID.
func (g ByIDGetter) Table(ctx context.Context, id descpb.ID) (catalog.TableDescriptor, error) {
	desc, err := g.Desc(ctx, id)
	if err != nil {
		if errors.Is(err, catalog.ErrDescriptorNotFound) {
			return nil, sqlerrors.NewUndefinedRelationError(&tree.TableRef{TableID: int64(id)})
		}
		return nil, err
	}
	tbl, ok := desc.(catalog.TableDescriptor)
	if !ok {
		return nil, sqlerrors.NewUndefinedRelationError(&tree.TableRef{TableID: int64(id)})
	}
	return tbl, nil
}

// Type looks up an immutable type descriptor by ID.
func (g ByIDGetter) Type(ctx context.Context, id descpb.ID) (catalog.TypeDescriptor, error) {
	desc, err := g.Desc(ctx, id)
	if err != nil {
		if errors.Is(err, catalog.ErrDescriptorNotFound) {
			return nil, pgerror.Newf(
				pgcode.UndefinedObject, "type with ID %d does not exist", id)
		}
		return nil, err
	}
	switch t := desc.(type) {
	case catalog.TypeDescriptor:
		return t, nil
	case catalog.TableDescriptor:
		// A given table name can resolve to either a type descriptor or a table
		// descriptor, because every table descriptor also defines an implicit
		// record type with the same name as the table...
		if g.flags.isMutable {
			// ...except if the type descriptor needs to be mutable.
			// We don't have the capability of returning a mutable type
			// descriptor for a table's implicit record type.
			return nil, errors.Wrapf(ErrMutableTableImplicitType,
				"cannot modify table record type %q", t.GetName())
		}
		return typedesc.CreateImplicitRecordTypeFromTableDesc(t)
	}
	return nil, pgerror.Newf(
		pgcode.UndefinedObject, "type with ID %d does not exist", id)
}

// Function looks up an immutable function descriptor by ID.
func (g ByIDGetter) Function(
	ctx context.Context, id descpb.ID,
) (catalog.FunctionDescriptor, error) {
	desc, err := g.Desc(ctx, id)
	if err != nil {
		if errors.Is(err, catalog.ErrDescriptorNotFound) {
			return nil, errors.Wrapf(tree.ErrFunctionUndefined, "function %d does not exist", id)
		}
		return nil, err
	}
	fn, ok := desc.(catalog.FunctionDescriptor)
	if !ok {
		return nil, errors.Wrapf(tree.ErrFunctionUndefined, "function %d does not exist", id)
	}
	return fn, nil
}

// MutableByIDGetter looks up mutable descriptors by ID.
type MutableByIDGetter getterBase

// AsByIDGetter returns this object as a ByIDGetter, which performs in
// exactly the same way except for the return types.
func (g MutableByIDGetter) AsByIDGetter() ByIDGetter {
	return ByIDGetter(g)
}

// Descs looks up mutable descriptors by ID.
func (g MutableByIDGetter) Descs(
	ctx context.Context, ids []descpb.ID,
) ([]catalog.MutableDescriptor, error) {
	descs, err := g.AsByIDGetter().Descs(ctx, ids)
	if err != nil {
		return nil, err
	}
	ret := make([]catalog.MutableDescriptor, len(descs))
	for i, desc := range descs {
		ret[i] = desc.(catalog.MutableDescriptor)
	}
	return ret, err
}

// Desc looks up a mutable descriptor by ID.
func (g MutableByIDGetter) Desc(
	ctx context.Context, id descpb.ID,
) (catalog.MutableDescriptor, error) {
	desc, err := g.AsByIDGetter().Desc(ctx, id)
	if err != nil {
		return nil, err
	}
	return desc.(catalog.MutableDescriptor), nil
}

// Database looks up a mutable database descriptor by ID.
func (g MutableByIDGetter) Database(ctx context.Context, id descpb.ID) (*dbdesc.Mutable, error) {
	db, err := g.AsByIDGetter().Database(ctx, id)
	if err != nil {
		return nil, err
	}
	return db.(*dbdesc.Mutable), nil
}

// Schema looks up a mutable schema descriptor by ID.
func (g MutableByIDGetter) Schema(ctx context.Context, id descpb.ID) (*schemadesc.Mutable, error) {
	sc, err := g.AsByIDGetter().Schema(ctx, id)
	if err != nil {
		return nil, err
	}
	return sc.(*schemadesc.Mutable), nil
}

// Table looks up a mutable table descriptor by ID.
func (g MutableByIDGetter) Table(ctx context.Context, id descpb.ID) (*tabledesc.Mutable, error) {
	tbl, err := g.AsByIDGetter().Table(ctx, id)
	if err != nil {
		return nil, err
	}
	return tbl.(*tabledesc.Mutable), nil
}

// Type looks up a mutable type descriptor by ID.
func (g MutableByIDGetter) Type(ctx context.Context, id descpb.ID) (*typedesc.Mutable, error) {
	typ, err := g.AsByIDGetter().Type(ctx, id)
	if err != nil {
		return nil, err
	}
	return typ.(*typedesc.Mutable), nil
}

// Function looks up a mutable function descriptor by ID.
func (g MutableByIDGetter) Function(ctx context.Context, id descpb.ID) (*funcdesc.Mutable, error) {
	fn, err := g.AsByIDGetter().Function(ctx, id)
	if err != nil {
		return nil, err
	}
	return fn.(*funcdesc.Mutable), nil
}

// ByNameGetter looks up immutable descriptors by name.
type ByNameGetter getterBase

// Database looks up an immutable database descriptor by name.
func (g ByNameGetter) Database(
	ctx context.Context, name string,
) (catalog.DatabaseDescriptor, error) {
	desc, err := getDescriptorByName(
		ctx, g.KV(), g.Descriptors(), nil /* db */, nil /* sc */, name, g.flags, catalog.Database,
	)
	if err != nil {
		return nil, err
	}
	if desc == nil {
		if g.flags.isOptional {
			return nil, nil
		}
		return nil, sqlerrors.NewUndefinedDatabaseError(name)
	}
	db, ok := desc.(catalog.DatabaseDescriptor)
	if !ok {
		if g.flags.isOptional {
			return nil, nil
		}
		return nil, sqlerrors.NewUndefinedDatabaseError(name)
	}
	return db, nil
}

// Schema looks up an immutable schema descriptor by name.
func (g ByNameGetter) Schema(
	ctx context.Context, db catalog.DatabaseDescriptor, name string,
) (catalog.SchemaDescriptor, error) {
	desc, err := getDescriptorByName(
		ctx, g.KV(), g.Descriptors(), db, nil /* sc */, name, g.flags, catalog.Schema,
	)
	if err != nil {
		return nil, err
	}
	if desc == nil {
		if g.flags.isOptional {
			return nil, nil
		}
		return nil, sqlerrors.NewUndefinedSchemaError(name)
	}
	schema, ok := desc.(catalog.SchemaDescriptor)
	if !ok {
		if g.flags.isOptional {
			return nil, nil
		}
		return nil, sqlerrors.NewUndefinedSchemaError(name)
	}
	return schema, nil
}

// Table looks up an immutable table descriptor by name.
func (g ByNameGetter) Table(
	ctx context.Context, db catalog.DatabaseDescriptor, sc catalog.SchemaDescriptor, name string,
) (catalog.TableDescriptor, error) {
	desc, err := getDescriptorByName(
		ctx, g.KV(), g.Descriptors(), db, sc, name, g.flags, catalog.Table,
	)
	if err != nil {
		return nil, err
	}
	if desc == nil {
		if g.flags.isOptional {
			return nil, nil
		}
		tn := tree.MakeTableNameWithSchema(
			tree.Name(db.GetName()), tree.Name(sc.GetName()), tree.Name(name),
		)
		return nil, sqlerrors.NewUndefinedRelationError(&tn)
	}
	return catalog.AsTableDescriptor(desc)
}

// Type looks up an immutable type descriptor by name.
func (g ByNameGetter) Type(
	ctx context.Context, db catalog.DatabaseDescriptor, sc catalog.SchemaDescriptor, name string,
) (catalog.TypeDescriptor, error) {
	desc, err := getDescriptorByName(ctx, g.KV(), g.Descriptors(), db, sc, name, g.flags, catalog.Any)
	if err != nil {
		return nil, err
	}
	if desc == nil {
		if g.flags.isOptional {
			return nil, nil
		}
		tn := tree.MakeTableNameWithSchema(
			tree.Name(db.GetName()), tree.Name(sc.GetName()), tree.Name(name),
		)
		return nil, sqlerrors.NewUndefinedRelationError(&tn)
	}
	if tbl, ok := desc.(catalog.TableDescriptor); ok {
		// A given table name can resolve to either a type descriptor or a table
		// descriptor, because every table descriptor also defines an implicit
		// record type with the same name as the table...
		if g.flags.isMutable {
			// ...except if the type descriptor needs to be mutable.
			// We don't have the capability of returning a mutable type
			// descriptor for a table's implicit record type.
			return nil, pgerror.Newf(pgcode.InsufficientPrivilege,
				"cannot modify table record type %q", name)
		}
		return typedesc.CreateImplicitRecordTypeFromTableDesc(tbl)
	}
	return catalog.AsTypeDescriptor(desc)
}

// MutableByNameGetter looks up mutable descriptors by name.
type MutableByNameGetter getterBase

// AsByNameGetter returns this object as a ByNameGetter, which performs in
// exactly the same way except for the return types.
func (g MutableByNameGetter) AsByNameGetter() ByNameGetter {
	return ByNameGetter(g)
}

// Database looks up a mutable database descriptor by name.
func (g MutableByNameGetter) Database(ctx context.Context, name string) (*dbdesc.Mutable, error) {
	db, err := g.AsByNameGetter().Database(ctx, name)
	if err != nil || db == nil {
		return nil, err
	}
	return db.(*dbdesc.Mutable), nil
}

// Schema looks up a mutable schema descriptor by name.
func (g MutableByNameGetter) Schema(
	ctx context.Context, db catalog.DatabaseDescriptor, name string,
) (*schemadesc.Mutable, error) {
	sc, err := g.AsByNameGetter().Schema(ctx, db, name)
	if err != nil || sc == nil {
		return nil, err
	}
	return sc.(*schemadesc.Mutable), nil
}

// Table looks up a mutable table descriptor by name.
func (g MutableByNameGetter) Table(
	ctx context.Context, db catalog.DatabaseDescriptor, sc catalog.SchemaDescriptor, name string,
) (*tabledesc.Mutable, error) {
	tbl, err := g.AsByNameGetter().Table(ctx, db, sc, name)
	if err != nil || tbl == nil {
		return nil, err
	}
	return tbl.(*tabledesc.Mutable), nil
}

// Type looks up a mutable type descriptor by name.
func (g MutableByNameGetter) Type(
	ctx context.Context, db catalog.DatabaseDescriptor, sc catalog.SchemaDescriptor, name string,
) (*typedesc.Mutable, error) {
	typ, err := g.AsByNameGetter().Type(ctx, db, sc, name)
	if err != nil || typ == nil {
		return nil, err
	}
	return typ.(*typedesc.Mutable), nil
}

func makeGetterBase(txn *kv.Txn, col *Collection, flags getterFlags) getterBase {
	return getterBase{
		txn:   &txnWrapper{Txn: txn, Collection: col},
		flags: flags,
	}
}

type getterBase struct {
	txn
	flags getterFlags
}

type (
	txn interface {
		KV() *kv.Txn
		Descriptors() *Collection
	}
	txnWrapper struct {
		*kv.Txn
		*Collection
	}
)

var _ txn = &txnWrapper{}

func (w *txnWrapper) KV() *kv.Txn {
	return w.Txn
}

func (w *txnWrapper) Descriptors() *Collection {
	return w.Collection
}

// getterFlags are the flags which power by-ID and by-name descriptor lookups
// in the Collection. The zero value of this struct is not a sane default.
//
// In any case, for historical reasons, some flags get overridden in
// inconsistent and sometimes bizarre ways depending on how the descriptors
// are looked up.
// TODO(postamar): clean up inconsistencies, enforce sane defaults.
type getterFlags struct {
	contextFlags
	layerFilters layerFilters
	descFilters  descFilters
}

type contextFlags struct {
	// isOptional specifies that the descriptor is being looked up on
	// a best-effort basis.
	//
	// Presently, for historical reasons, this is overridden to true for
	// all mutable by-ID lookups, and for all immutable by-ID object lookups.
	// TODO(postamar): clean this up
	isOptional bool
	// isMutable specifies that a mutable descriptor is to be returned.
	isMutable bool
}

type layerFilters struct {
	// withoutSynthetic specifies bypassing the synthetic descriptor layer.
	withoutSynthetic bool
	// withoutLeased specifies bypassing the leased descriptor layer.
	withoutLeased bool
	// withoutStorage specifies avoiding any queries to the KV layer.
	withoutStorage bool
	// withoutHydration specifies avoiding hydrating the descriptors.
	// This can be set to true only when looking up descriptors when hydrating
	// another group of descriptors. The purpose is to avoid potential infinite
	// recursion loop when trying to hydrate a descriptor which would lead to
	// the hydration of another descriptor which depends on it.
	// TODO(postamar): untangle the hydration mess
	withoutHydration bool
}

type descFilters struct {
	// withoutDropped specifies to raise an error if the looked-up descriptor
	// is in the DROP state.
	//
	// Presently, for historical reasons, this is overridden everywhere except
	// for immutable by-ID lookups: to true for by-name lookups and to false for
	// mutable by-ID lookups.
	// TODO(postamar): clean this up
	withoutDropped bool
	// withoutOffline specifies to raise an error if the looked-up descriptor
	// is in the OFFLINE state.
	//
	// Presently, for historical reasons, this is overridden to true for mutable
	// by-ID lookups.
	// TODO(postamar): clean this up
	withoutOffline bool
	// withoutCommittedAdding specifies if committed descriptors in the
	// adding state will be ignored.
	withoutCommittedAdding bool
	// maybeParentID specifies, when set, that the looked-up descriptor
	// should have the same parent ID, when set.
	maybeParentID descpb.ID
}

func fromCommonFlags(flags tree.CommonLookupFlags) (f getterFlags) {
	return getterFlags{
		contextFlags: contextFlags{
			isOptional: !flags.Required,
			isMutable:  flags.RequireMutable,
		},
		layerFilters: layerFilters{
			withoutSynthetic: flags.AvoidSynthetic,
			withoutLeased:    flags.AvoidLeased,
		},
		descFilters: descFilters{
			withoutDropped: !flags.IncludeDropped,
			withoutOffline: !flags.IncludeOffline,
			maybeParentID:  flags.ParentID,
		},
	}
}

func fromObjectFlags(flags tree.ObjectLookupFlags) getterFlags {
	return fromCommonFlags(flags.CommonLookupFlags)
}

func defaultFlags() getterFlags {
	return fromCommonFlags(tree.CommonLookupFlags{})
}

func defaultUnleasedFlags() (f getterFlags) {
	f.layerFilters.withoutLeased = true
	return f
}

// ByID returns a ByIDGetterBuilder.
func (tc *Collection) ByID(txn *kv.Txn) ByIDGetterBuilder {
	return ByIDGetterBuilder(makeGetterBase(txn, tc, defaultFlags()))
}

// ByIDGetterBuilder is a builder object for ByIDGetter and MutableByIDGetter.
type ByIDGetterBuilder getterBase

// WithFlags configures the ByIDGetterBuilder with the given flags.
func (b ByIDGetterBuilder) WithFlags(flags tree.CommonLookupFlags) ByIDGetterBuilder {
	b.flags = fromCommonFlags(flags)
	return b
}

// WithObjFlags configures the ByIDGetterBuilder with the given object flags.
func (b ByIDGetterBuilder) WithObjFlags(flags tree.ObjectLookupFlags) ByIDGetterBuilder {
	b.flags = fromObjectFlags(flags)
	return b
}

// Mutable builds a MutableByIDGetter.
func (b ByIDGetterBuilder) Mutable() MutableByIDGetter {
	b.flags.isOptional = false
	b.flags.isMutable = true
	b.flags.layerFilters.withoutLeased = true
	b.flags.descFilters.withoutDropped = false
	b.flags.descFilters.withoutOffline = false
	return MutableByIDGetter(b)
}

// Immutable builds a ByIDGetter.
func (b ByIDGetterBuilder) Immutable() ByIDGetter {
	if b.flags.isMutable {
		b.flags.layerFilters.withoutLeased = true
		b.flags.isMutable = false
	}
	return ByIDGetter(b)
}

// ByName returns a ByNameGetterBuilder.
func (tc *Collection) ByName(txn *kv.Txn) ByNameGetterBuilder {
	return ByNameGetterBuilder(makeGetterBase(txn, tc, defaultFlags()))
}

// ByNameGetterBuilder is a builder object for ByNameGetter and MutableByNameGetter.
type ByNameGetterBuilder getterBase

// WithFlags configures the ByIDGetterBuilder with the given flags.
func (b ByNameGetterBuilder) WithFlags(flags tree.CommonLookupFlags) ByNameGetterBuilder {
	b.flags = fromCommonFlags(flags)
	return b
}

// WithObjFlags configures the ByNameGetterBuilder with the given object flags.
func (b ByNameGetterBuilder) WithObjFlags(flags tree.ObjectLookupFlags) ByNameGetterBuilder {
	b.flags = fromObjectFlags(flags)
	return b
}

// Mutable builds a MutableByNameGetter.
func (b ByNameGetterBuilder) Mutable() MutableByNameGetter {
	b.flags.isMutable = true
	b.flags.layerFilters.withoutLeased = true
	b.flags.descFilters.withoutDropped = true
	return MutableByNameGetter(b)
}

// Immutable builds a ByNameGetter.
func (b ByNameGetterBuilder) Immutable() ByNameGetter {
	if b.flags.isMutable {
		b.flags.layerFilters.withoutLeased = true
		b.flags.isMutable = false
	}
	b.flags.descFilters.withoutDropped = true
	return ByNameGetter(b)
}
