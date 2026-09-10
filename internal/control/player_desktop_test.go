package control

import (
	"context"
	"testing"

	"github.com/nodelane/nodelane-room/internal/model"
	"github.com/nodelane/nodelane-room/internal/pki"
)

func TestOwnerManagementAfterLeaving(t *testing.T) {
	s, ca := database(t)
	one := apiServer(t, s, ca)
	two := apiServer(t, &Store{Pool: s.Pool, Network: s.Network}, ca)
	owner, guest, stranger := user(t, one, "owner"), user(t, two, "guest"), user(t, two, "stranger")
	ctx := context.Background()
	room := create(t, owner)
	join(t, guest, room)
	path := "/v2/rooms/" + room.Room.ID
	must(t, owner.Call(ctx, "POST", path+"/leave", struct{}{}, nil))
	var list []model.Room
	must(t, owner.Call(ctx, "GET", "/v2/rooms", nil, &list))
	if len(list) != 1 || list[0].ID != room.Room.ID {
		t.Fatal("left room not available for its owner")
	}
	var view model.RoomManagement
	must(t, owner.Call(ctx, "GET", path+"/manage", nil, &view))
	if len(view.Members) != 1 || view.Members[0].DeviceID != guest.Identity.ID() || view.Game.ID != "custom" {
		t.Fatal("incorrect management view")
	}
	statusError(t, stranger.Call(ctx, "GET", path+"/manage", nil, &view), 403)
	statusError(t, guest.Call(ctx, "GET", path+"/manage", nil, &view), 403)
	_, pub, err := pki.TunnelKey()
	must(t, err)
	statusError(t, owner.Call(ctx, "POST", path+"/lease", model.LeaseRequest{PublicKey: pub}, nil), 403)
	var snap model.Snapshot
	must(t, owner.Call(ctx, "GET", path, nil, &snap))
	if len(snap.Members) != 0 || len(snap.Endpoints) != 0 {
		t.Fatal("management granted network authority")
	}
	must(t, owner.Call(ctx, "POST", path+"/transfer", model.MemberRequest{DeviceID: guest.Identity.ID()}, nil))
	statusError(t, owner.Call(ctx, "GET", path+"/manage", nil, &view), 403)
	must(t, owner.Call(ctx, "GET", "/v2/rooms", nil, &list))
	if len(list) != 0 {
		t.Fatal("former owner still lists room")
	}
	must(t, guest.Call(ctx, "GET", "/v2/rooms", nil, &list))
	if len(list) != 1 {
		t.Fatal("new owner cannot list room")
	}
	must(t, guest.Call(ctx, "POST", path+"/close", struct{}{}, nil))
	must(t, guest.Call(ctx, "GET", "/v2/rooms", nil, &list))
	if len(list) != 0 {
		t.Fatal("closed room still listed")
	}
}
