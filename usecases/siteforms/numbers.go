package siteforms

import (
	"context"
	"errors"

	"vozko/domain/shared"
	uw "vozko/domain/unofficial_whatsapp"
	businessphone "vozko/domain/whatsapp/business_phone"
)

type BusinessPhones interface {
	FindByID(id string) (*businessphone.WhatsAppBusinessPhoneNumber, error)
}

type Instances interface {
	FindByID(ctx context.Context, id string) (*uw.Instance, error)
}

type PageNumbers struct {
	Phones    BusinessPhones
	Grants    businessphone.AccessGrantReader
	Instances Instances
}

func (n PageNumbers) NumberOf(ctx context.Context, workspaceID, ref string) (string, error) {
	phone, err := n.Phones.FindByID(ref)
	switch {
	case err == nil && phone != nil:
		allowed, err := businessphone.CanWorkspaceSendFrom(workspaceID, ref, phone, n.Grants)
		if err != nil {
			return "", err
		}
		return pageNumberDigits(allowed, phone.DisplayPhoneNumber)
	case err != nil && !errors.Is(err, businessphone.ErrPhoneNumberNotFound):
		return "", err
	}
	instance, err := n.Instances.FindByID(ctx, ref)
	switch {
	case errors.Is(err, uw.ErrInstanceNotFound):
		return "", ErrWhatsAppNumber
	case err != nil:
		return "", err
	case instance == nil:
		return "", ErrWhatsAppNumber
	}
	return pageNumberDigits(instance.WorkspaceID == workspaceID, instance.PhoneNumber)
}

func pageNumberDigits(allowed bool, raw string) (string, error) {
	digits := shared.BrazilPhoneDigits(raw)
	if !allowed || digits == "" {
		return "", ErrWhatsAppNumber
	}
	return digits, nil
}
