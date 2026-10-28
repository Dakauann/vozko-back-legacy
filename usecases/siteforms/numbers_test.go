package siteforms

import (
	"context"
	"errors"
	"testing"

	uw "vozko/domain/unofficial_whatsapp"
	businessphone "vozko/domain/whatsapp/business_phone"
)

type phoneShelf struct {
	rows map[string]*businessphone.WhatsAppBusinessPhoneNumber
	err  error
}

func (p phoneShelf) FindByID(id string) (*businessphone.WhatsAppBusinessPhoneNumber, error) {
	if p.err != nil {
		return nil, p.err
	}
	if row, ok := p.rows[id]; ok {
		return row, nil
	}
	return nil, businessphone.ErrPhoneNumberNotFound
}

type grantShelf struct {
	granted map[string]bool
	err     error
}

func (g grantShelf) HasAccess(workspaceID, phoneID string) (bool, error) {
	return g.granted[workspaceID+"|"+phoneID], g.err
}

type instanceShelf struct {
	rows map[string]*uw.Instance
	err  error
}

func (i instanceShelf) FindByID(_ context.Context, id string) (*uw.Instance, error) {
	if i.err != nil {
		return nil, i.err
	}
	if row, ok := i.rows[id]; ok {
		return row, nil
	}
	return nil, uw.ErrInstanceNotFound
}

func TestAPageUsesOnlyANumberItsWorkspaceMaySendFrom(t *testing.T) {
	phones := phoneShelf{rows: map[string]*businessphone.WhatsAppBusinessPhoneNumber{
		"own":     {ID: "own", OwnerWorkspaceID: "w1", DisplayPhoneNumber: "+55 84 99999-0000"},
		"foreign": {ID: "foreign", OwnerWorkspaceID: "w2", DisplayPhoneNumber: "+55 84 98888-0000"},
		"shared":  {ID: "shared", DisplayPhoneNumber: "(11) 97777-0000"},
		"abroad":  {ID: "abroad", OwnerWorkspaceID: "w1", DisplayPhoneNumber: "+1 555 0100"},
	}}
	grants := grantShelf{granted: map[string]bool{"w1|shared": true}}
	instances := instanceShelf{rows: map[string]*uw.Instance{
		"inst-own":     {ID: "inst-own", WorkspaceID: "w1", PhoneNumber: "5521966660000"},
		"inst-foreign": {ID: "inst-foreign", WorkspaceID: "w2", PhoneNumber: "5521955550000"},
	}}
	numbers := PageNumbers{Phones: phones, Grants: grants, Instances: instances}
	cases := map[string]struct {
		ref  string
		want string
		err  error
	}{
		"own official number":        {ref: "own", want: "5584999990000"},
		"granted shared number":      {ref: "shared", want: "5511977770000"},
		"own unofficial instance":    {ref: "inst-own", want: "5521966660000"},
		"another workspace phone":    {ref: "foreign", err: ErrWhatsAppNumber},
		"another workspace line":     {ref: "inst-foreign", err: ErrWhatsAppNumber},
		"number of the wrong length": {ref: "abroad", err: ErrWhatsAppNumber},
		"unknown everywhere":         {ref: "nothing", err: ErrWhatsAppNumber},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			got, err := numbers.NumberOf(context.Background(), "w1", tc.ref)
			if got != tc.want || !errors.Is(err, tc.err) {
				t.Fatalf("got %q %v, want %q %v", got, err, tc.want, tc.err)
			}
		})
	}
}

func TestNumberLookupFailuresAreNeverSwallowed(t *testing.T) {
	down := errors.New("db down")
	cases := map[string]PageNumbers{
		"phones down":    {Phones: phoneShelf{err: down}, Grants: grantShelf{}, Instances: instanceShelf{}},
		"grants down":    {Phones: phoneShelf{rows: map[string]*businessphone.WhatsAppBusinessPhoneNumber{"n": {ID: "n", DisplayPhoneNumber: "5584999990000"}}}, Grants: grantShelf{err: down}, Instances: instanceShelf{}},
		"instances down": {Phones: phoneShelf{}, Grants: grantShelf{}, Instances: instanceShelf{err: down}},
	}
	for name, numbers := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := numbers.NumberOf(context.Background(), "w1", "n")
			if !errors.Is(err, down) || errors.Is(err, ErrWhatsAppNumber) {
				t.Fatalf("err = %v, want the store failure", err)
			}
		})
	}
}
