package sightmap

import "testing"

func TestRecordMatches_CarryIDs(t *testing.T) {
	c := &Corpus{
		Requests: []RequestDef{{ID: "req00001", Name: "CreateOrder", Route: "/api/orders", Method: "POST"}},
		Messages: []MessageDef{{ID: "msg00001", Name: "CartVersion", Message: "cart version"}},
	}
	if got := c.RequestsForRecord(Request{URL: "https://shop.example.com/api/orders", Method: "POST"}); len(got) != 1 || got[0].ID != "req00001" {
		t.Errorf("RequestsForRecord = %+v, want one match with id req00001", got)
	}
	if got := c.MessagesForRecord(Message{Level: "warn", Text: "cart version mismatch"}); len(got) != 1 || got[0].ID != "msg00001" {
		t.Errorf("MessagesForRecord = %+v, want one match with id msg00001", got)
	}
}
