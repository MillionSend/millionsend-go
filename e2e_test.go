package millionsend

import (
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestE2EAudienceContactLifecycle exercises the audience + contact lifecycle
// against a real MillionSend instance. It runs only when MILLIONSEND_API_KEY is
// set (and MILLIONSEND_BASE_URL if not localhost:3001); otherwise it skips. No
// verified sender domain is required.
func TestE2EAudienceContactLifecycle(t *testing.T) {
	if os.Getenv("MILLIONSEND_API_KEY") == "" {
		t.Skip("set MILLIONSEND_API_KEY to run the e2e test")
	}
	c := NewClient("") // reads key + base URL from the environment

	aud, err := c.Audiences.Create(&CreateAudienceRequest{Name: fmt.Sprintf("sdk-e2e-%d", time.Now().UnixNano())})
	require.NoError(t, err)
	require.NotEmpty(t, aud.Id)
	t.Cleanup(func() { _, _ = c.Audiences.Remove(aud.Id) })

	email := fmt.Sprintf("sdk-e2e-%d@example.com", time.Now().UnixNano())
	created, err := c.Contacts.Create(&CreateContactRequest{AudienceId: aud.Id, Email: email, FirstName: "Ada"})
	require.NoError(t, err)
	require.NotEmpty(t, created.Id)

	got, err := c.Contacts.Get(ContactAddress{AudienceId: aud.Id, Email: email})
	require.NoError(t, err)
	assert.Equal(t, email, got.Email)
	assert.Equal(t, "Ada", got.FirstName)

	_, err = c.Contacts.Update(&UpdateContactRequest{AudienceId: aud.Id, Email: email, Unsubscribed: Ptr(true)})
	require.NoError(t, err)

	removed, err := c.Contacts.Remove(ContactAddress{AudienceId: aud.Id, Email: email})
	require.NoError(t, err)
	assert.True(t, removed.Deleted)

	// A missing contact surfaces as a typed error, never a panic.
	_, err = c.Contacts.Get(ContactAddress{Email: "does-not-exist@example.com"})
	require.Error(t, err)
	var mse *MillionSendError
	require.ErrorAs(t, err, &mse)
	assert.Equal(t, "not_found", mse.Name)
}
