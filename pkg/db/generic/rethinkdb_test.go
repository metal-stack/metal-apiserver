package generic_test

import (
	"log/slog"
	"testing"

	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	"github.com/metal-stack/metal-apiserver/pkg/db/metal"
	"github.com/metal-stack/metal-apiserver/pkg/db/queries"
	"github.com/metal-stack/metal-apiserver/pkg/errorutil"
	"github.com/metal-stack/metal-apiserver/pkg/test"
	"github.com/metal-stack/metal-lib/pkg/pointer"
	"github.com/stretchr/testify/require"
)

func TestGenericCRUD(t *testing.T) {
	log := slog.Default()

	ds, _, rethinkCloser := test.StartRethink(t, log)
	defer func() {
		rethinkCloser()
	}()

	ctx := t.Context()

	nonexisting, err := ds.IP().Get(ctx, "1.2.3.4")
	require.Nil(t, nonexisting)
	require.Error(t, err)
	require.EqualError(t, err, errorutil.NotFound("no ip with id \"1.2.3.4\" found").Error())

	created, err := ds.IP().Create(ctx, &metal.IP{IPAddress: "1.2.3.4"})
	require.NoError(t, err)
	require.NotNil(t, created)
	require.Equal(t, "1.2.3.4", created.IPAddress)
	require.NotNil(t, created.Created)

	created.Description = "Modified IP"
	err = ds.IP().Update(ctx, created)
	require.NoError(t, err)

	updated, err := ds.IP().Get(ctx, "1.2.3.4")
	require.NoError(t, err)
	require.NotNil(t, updated)
	require.Equal(t, "1.2.3.4", updated.IPAddress)
	require.Equal(t, "Modified IP", updated.Description)
	require.NotNil(t, updated.Changed)

	// Check Unique
	err = ds.IP().CheckAlreadyExists(ctx, "1.2.3.4")
	require.EqualError(t, err, errorutil.Conflict("ip with id:1.2.3.4 already exists").Error())

	err = ds.IP().CheckAlreadyExists(ctx, "1.2.3.5")
	require.NoError(t, err)

	// Delete does not give a notfound
	err = ds.IP().Delete(ctx, &metal.IP{IPAddress: "1.2.3.5"})
	require.NoError(t, err)

	err = ds.IP().Delete(ctx, &metal.IP{IPAddress: "1.2.3.4"})
	require.NoError(t, err)
}

func TestFindAndListGeneric(t *testing.T) {
	log := slog.Default()

	ds, _, rethinkCloser := test.StartRethink(t, log)
	defer func() {
		rethinkCloser()
	}()

	ctx := t.Context()

	created, err := ds.IP().Create(ctx, &metal.IP{IPAddress: "1.2.3.4", ProjectID: "p1"})
	require.NoError(t, err)
	require.NotNil(t, created)
	require.Equal(t, "1.2.3.4", created.IPAddress)
	require.Equal(t, "p1", created.ProjectID)
	require.NotNil(t, created.Created)

	createdAlreadyExist, err := ds.IP().Create(ctx, &metal.IP{IPAddress: "1.2.3.4", ProjectID: "p1"})
	require.Nil(t, createdAlreadyExist)
	require.Error(t, err)
	require.EqualError(t, err, errorutil.Conflict("cannot create ip in database, entity already exists: 1.2.3.4").Error())

	created2, err := ds.IP().Create(ctx, &metal.IP{IPAddress: "1.2.3.2", ProjectID: "p1"})
	require.NoError(t, err)
	require.NotNil(t, created2)
	require.Equal(t, "1.2.3.2", created2.IPAddress)
	require.Equal(t, "p1", created2.ProjectID)
	require.NotNil(t, created2.Created)

	found, err := ds.IP().Find(ctx, queries.IpFilter(&apiv2.IPQuery{Ip: pointer.Pointer("1.2.3.4"), Project: pointer.Pointer("p1")}))
	require.NoError(t, err)
	require.NotNil(t, found)
	require.Equal(t, "1.2.3.4", found.IPAddress)

	notfound, err := ds.IP().Find(ctx, queries.IpFilter(&apiv2.IPQuery{Ip: pointer.Pointer("1.2.3.5")}))
	require.Nil(t, notfound)
	require.Error(t, err)
	require.EqualError(t, err, errorutil.NotFound("no ip found").Error())

	moreThanOneFound, err := ds.IP().Find(ctx, queries.IpFilter(&apiv2.IPQuery{Project: pointer.Pointer("p1")}))
	require.Nil(t, moreThanOneFound)
	require.Error(t, err)
	require.EqualError(t, err, "more than one ip exists")

	listOnlyOne, err := ds.IP().List(ctx, queries.IpFilter(&apiv2.IPQuery{Ip: pointer.Pointer("1.2.3.4"), Project: pointer.Pointer("p1")}))
	require.NoError(t, err)
	require.NotNil(t, listOnlyOne)
	require.Len(t, listOnlyOne, 1)

	listBoth, err := ds.IP().List(ctx, queries.IpFilter(&apiv2.IPQuery{Project: pointer.Pointer("p1")}))
	require.NoError(t, err)
	require.NotNil(t, listBoth)
	require.Len(t, listBoth, 2)

	listWithNilQuery, err := ds.IP().List(ctx, nil)
	require.NoError(t, err)
	require.NotNil(t, listWithNilQuery)
	require.Len(t, listWithNilQuery, 2)
}
