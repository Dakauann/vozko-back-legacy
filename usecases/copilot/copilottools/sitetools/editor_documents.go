package sitetools

import (
	"context"
	"errors"

	"vozko/domain/copilot"
	"vozko/domain/strictjson"
	"vozko/usecases/copilot/copilottools/screencall"
	"vozko/usecases/siteinspect"
)

type inspectOrder struct {
	PreviewURL string   `json:"preview_url"`
	Routes     []string `json:"routes"`
}

type inspectReply struct {
	Documents map[string]string `json:"documents"`
}

type editorDocuments struct {
	cc copilot.Context
}

func (e editorDocuments) Documents(ctx context.Context, req siteinspect.DocumentRequest) (map[string]string, error) {
	if e.cc.Screen == nil {
		return nil, errors.New(siteNoEditor)
	}
	reply, err := screencall.Run(ctx, e.cc, copilot.ScreenInspect, inspectOrder{PreviewURL: req.PreviewURL, Routes: req.Routes}, editorWords)
	if err != nil {
		return nil, err
	}
	var got inspectReply
	if strictjson.Decode(reply.Data, &got) != nil || got.Documents == nil {
		return nil, errors.New(siteUnreadable)
	}
	return got.Documents, nil
}
