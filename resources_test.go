package millionsend

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEmailsGetAndCancel(t *testing.T) {
	c, rec := mockServer(t, 200, `{"object":"email","id":"e1"}`)
	_, err := c.Emails.Get("e1")
	require.NoError(t, err)
	assert.Equal(t, http.MethodGet, rec.Method)
	assert.Equal(t, "/emails/e1", rec.Path)

	_, err = c.Emails.Cancel("e1")
	require.NoError(t, err)
	assert.Equal(t, http.MethodPost, rec.Method)
	assert.Equal(t, "/emails/e1/cancel", rec.Path)
}

func TestBatchSend(t *testing.T) {
	c, rec := mockServer(t, 200, `{"data":[{"id":"1"},{"id":"2"}]}`)
	res, err := c.Batch.Send([]*SendEmailRequest{
		{From: "a@x.dev", To: []string{"b@x.dev"}, Subject: "1", Text: "one"},
		{From: "a@x.dev", To: []string{"c@x.dev"}, Subject: "2", Text: "two"},
	}, WithIdempotencyKey("batch-1"))
	require.NoError(t, err)
	assert.Equal(t, "/emails/batch", rec.Path)
	assert.Equal(t, "batch-1", rec.Header.Get("Idempotency-Key"))
	var arr []any
	require.NoError(t, json.Unmarshal(rec.Body, &arr))
	assert.Len(t, arr, 2)
	assert.Len(t, res.Data, 2)
}

func TestContactsCreate(t *testing.T) {
	c, rec := mockServer(t, 200, `{"object":"contact","id":"c1"}`)
	_, err := c.Contacts.Create(&CreateContactRequest{Email: "c@x.dev", FirstName: "Ada"})
	require.NoError(t, err)
	assert.Equal(t, http.MethodPost, rec.Method)
	assert.Equal(t, "/contacts", rec.Path)
	b := bodyMap(t, rec)
	assert.Equal(t, "c@x.dev", b["email"])
	assert.Equal(t, "Ada", b["first_name"])
}

func TestContactsAddressing(t *testing.T) {
	c, rec := mockServer(t, 200, `{"object":"contact","id":"c1"}`)
	_, err := c.Contacts.Get(ContactAddress{Id: "c1"})
	require.NoError(t, err)
	assert.Equal(t, "/contacts/c1", rec.Path)

	_, err = c.Contacts.Get(ContactAddress{Email: "c@x.dev"})
	require.NoError(t, err)
	assert.Equal(t, "/contacts/c@x.dev", rec.Path)

	// Email wins when both are set.
	_, err = c.Contacts.Get(ContactAddress{Id: "c1", Email: "e@x.dev"})
	require.NoError(t, err)
	assert.Equal(t, "/contacts/e@x.dev", rec.Path)
}

func TestContactsUpdateSendsProvidedFields(t *testing.T) {
	c, rec := mockServer(t, 200, `{"object":"contact","id":"c1"}`)
	_, err := c.Contacts.Update(&UpdateContactRequest{Id: "c1", FirstName: Ptr("Bob"), Unsubscribed: Ptr(true)})
	require.NoError(t, err)
	assert.Equal(t, http.MethodPatch, rec.Method)
	assert.Equal(t, "/contacts/c1", rec.Path)
	b := bodyMap(t, rec)
	assert.Equal(t, "Bob", b["first_name"])
	assert.Equal(t, true, b["unsubscribed"])

	// A nil pointer is omitted; an explicit false is still sent.
	_, err = c.Contacts.Update(&UpdateContactRequest{Email: "c@x.dev", Unsubscribed: Ptr(false)})
	require.NoError(t, err)
	assert.Equal(t, "/contacts/c@x.dev", rec.Path)
	b = bodyMap(t, rec)
	assert.Equal(t, false, b["unsubscribed"])
	_, hasFirst := b["first_name"]
	assert.False(t, hasFirst)
}

func TestContactsRemoveAndList(t *testing.T) {
	c, rec := mockServer(t, 200, `{"object":"list","data":[],"has_more":false}`)
	_, err := c.Contacts.Remove(ContactAddress{Email: "c@x.dev"})
	require.NoError(t, err)
	assert.Equal(t, http.MethodDelete, rec.Method)
	assert.Equal(t, "/contacts/c@x.dev", rec.Path)

	_, err = c.Contacts.List(&ListOptions{After: "cur"})
	require.NoError(t, err)
	assert.Equal(t, "/contacts", rec.Path)
	assert.Equal(t, "after=cur", rec.RawQuery)
}

func TestContactsUpdateTopics(t *testing.T) {
	c, rec := mockServer(t, 200, `{"id":"c1"}`)
	_, err := c.Contacts.UpdateTopics(ContactAddress{Id: "c1"}, []ContactTopicUpdate{
		{Id: "t1", Subscription: "opt_out"},
	})
	require.NoError(t, err)
	assert.Equal(t, http.MethodPatch, rec.Method)
	assert.Equal(t, "/contacts/c1/topics", rec.Path)
	var arr []map[string]any
	require.NoError(t, json.Unmarshal(rec.Body, &arr))
	require.Len(t, arr, 1)
	assert.Equal(t, "t1", arr[0]["id"])
	assert.Equal(t, "opt_out", arr[0]["subscription"])
}

func TestTopicsCRUD(t *testing.T) {
	c, rec := mockServer(t, 200, `{"data":[]}`)
	_, err := c.Topics.Create(&CreateTopicRequest{Name: "Product", DefaultSubscription: "opt_in"})
	require.NoError(t, err)
	assert.Equal(t, "/topics", rec.Path)
	b := bodyMap(t, rec)
	assert.Equal(t, "Product", b["name"])
	assert.Equal(t, "opt_in", b["default_subscription"])

	_, err = c.Topics.Get("t1")
	require.NoError(t, err)
	assert.Equal(t, "/topics/t1", rec.Path)

	_, err = c.Topics.List()
	require.NoError(t, err)
	assert.Equal(t, http.MethodGet, rec.Method)
	assert.Equal(t, "/topics", rec.Path)

	_, err = c.Topics.Remove("t1")
	require.NoError(t, err)
	assert.Equal(t, http.MethodDelete, rec.Method)
}

func TestBroadcastsLifecycle(t *testing.T) {
	c, rec := mockServer(t, 200, `{"id":"b1"}`)
	_, err := c.Broadcasts.Create(&CreateBroadcastRequest{SegmentId: "s1", From: "a@x.dev", Subject: "News", Html: "<p>hi</p>"})
	require.NoError(t, err)
	assert.Equal(t, "/broadcasts", rec.Path)
	b := bodyMap(t, rec)
	assert.Equal(t, "s1", b["segment_id"])
	assert.Equal(t, "News", b["subject"])

	_, err = c.Broadcasts.Get("b1")
	require.NoError(t, err)
	assert.Equal(t, "/broadcasts/b1", rec.Path)

	_, err = c.Broadcasts.List(nil)
	require.NoError(t, err)
	assert.Equal(t, "/broadcasts", rec.Path)
	assert.Empty(t, rec.RawQuery)

	_, err = c.Broadcasts.Update("b1", &UpdateBroadcastRequest{Subject: "New"})
	require.NoError(t, err)
	assert.Equal(t, http.MethodPatch, rec.Method)
	assert.Equal(t, "/broadcasts/b1", rec.Path)

	_, err = c.Broadcasts.Send("b1", &SendBroadcastRequest{ScheduledAt: "2999-01-01T00:00:00Z"})
	require.NoError(t, err)
	assert.Equal(t, "/broadcasts/b1/send", rec.Path)
	assert.Equal(t, "2999-01-01T00:00:00Z", bodyMap(t, rec)["scheduled_at"])

	_, err = c.Broadcasts.Cancel("b1")
	require.NoError(t, err)
	assert.Equal(t, "/broadcasts/b1/cancel", rec.Path)

	_, err = c.Broadcasts.Remove("b1")
	require.NoError(t, err)
	assert.Equal(t, http.MethodDelete, rec.Method)
	assert.Equal(t, "/broadcasts/b1", rec.Path)
}

func TestSegmentsCRUD(t *testing.T) {
	c, rec := mockServer(t, 200, `{"object":"segment","id":"s1"}`)
	filter := SegmentFilter{Match: "all", Conditions: []SegmentCondition{{Field: "email", Op: "is_set"}}}
	_, err := c.Segments.Create(&CreateSegmentRequest{Name: "Active", Filter: filter})
	require.NoError(t, err)
	assert.Equal(t, "/segments", rec.Path)
	b := bodyMap(t, rec)
	assert.Equal(t, "Active", b["name"])
	require.Contains(t, b, "filter")

	_, err = c.Segments.Get("s1")
	require.NoError(t, err)
	assert.Equal(t, "/segments/s1", rec.Path)

	_, err = c.Segments.List(&ListOptions{Before: "cur"})
	require.NoError(t, err)
	assert.Equal(t, "/segments", rec.Path)
	assert.Equal(t, "before=cur", rec.RawQuery)

	_, err = c.Segments.Update("s1", &UpdateSegmentRequest{Name: "Renamed"})
	require.NoError(t, err)
	assert.Equal(t, http.MethodPatch, rec.Method)
	assert.Equal(t, "/segments/s1", rec.Path)

	_, err = c.Segments.Remove("s1")
	require.NoError(t, err)
	assert.Equal(t, http.MethodDelete, rec.Method)
	assert.Equal(t, "/segments/s1", rec.Path)
}
