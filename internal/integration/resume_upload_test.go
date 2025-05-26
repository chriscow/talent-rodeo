package integration

import (
	"os"
	"testing"
	"time"

	"github.com/playwright-community/playwright-go"
	"github.com/stretchr/testify/assert"
)

// TestResumeUploadSuccess tests the happy path of resume upload
func TestResumeUploadSuccess(t *testing.T) {
	t.Skip("this test needs to not fully evaluate the resume (or do it much faster)")

	it := NewIntegrationTest(t)
	defer it.Close()

	// Authenticate the test user
	it.Authenticate()

	it.NavigateTo("/profile/resume/upload")

	// Upload resume
	fileChooser, err := it.Page.ExpectFileChooser(func() error {
		return it.Page.Locator("input[type=file]").Click()
	})
	it.requireNoError(err, "Failed to get file chooser")

	err = fileChooser.SetFiles("../../testdata/sample-resumes/sample-resume.docx")
	it.requireNoError(err, "Failed to set file in file chooser")

	// Enter phone number
	err = it.Page.Locator("#phone").Fill("+15555555555")
	it.requireNoError(err, "Failed to enter phone number")

	// Accept terms
	err = it.Page.Locator("#terms").Click()
	it.requireNoError(err, "Failed to click terms checkbox")

	// Click submit and wait for form submission
	err = it.Page.Locator("button[type=submit]:text('Continue')").Click()
	it.requireNoError(err, "Failed to click submit button")

	// Wait for processing message
	err = it.Page.Locator("h1:text('Processing Your Resume')").WaitFor(playwright.LocatorWaitForOptions{
		State:   playwright.WaitForSelectorStateVisible,
		Timeout: playwright.Float(5000),
	})
	it.requireNoError(err, "Failed to see resume processing message")

	// Also wait for the subtext to confirm the page has fully loaded
	err = it.Page.Locator("text=Please wait while we analyze your resume").WaitFor(playwright.LocatorWaitForOptions{
		State:   playwright.WaitForSelectorStateVisible,
		Timeout: playwright.Float(300000),
	})
	it.requireNoError(err, "Failed to see resume analysis message")

	// Wait for navigation to schedule page (with a longer timeout)
	err = it.Page.WaitForURL("**/profile/schedule", playwright.PageWaitForURLOptions{
		WaitUntil: playwright.WaitUntilStateNetworkidle,
		Timeout:   playwright.Float(10000), // 10 seconds timeout
	})
	it.requireNoError(err, "Failed to wait for navigation to schedule page")

	// Verify we're on the schedule page
	currentURL := it.Page.URL()
	assert.Contains(t, currentURL, "/profile/schedule", "Should be redirected to schedule page")

	// Wait for schedule page content
	err = it.Page.Locator("text=Great! We've got your resume").WaitFor(playwright.LocatorWaitForOptions{
		State:   playwright.WaitForSelectorStateVisible,
		Timeout: playwright.Float(5000),
	})
	it.requireNoError(err, "Failed to wait for schedule page heading")
}

// TestResumeUploadNoPhone tests the validation when phone number is missing
func TestResumeUploadNoPhone(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping test in short mode")
	}

	it := NewIntegrationTest(t)
	defer it.Close()

	// Authenticate the test user
	it.Authenticate()

	it.NavigateTo("/profile/resume/upload")

	// Upload resume
	fileChooser, err := it.Page.ExpectFileChooser(func() error {
		return it.Page.Locator("input[type=file]").Click()
	})
	it.requireNoError(err, "Failed to get file chooser")

	err = fileChooser.SetFiles("../../testdata/sample-resumes/sample-resume.docx")
	it.requireNoError(err, "Failed to set file in file chooser")

	// Accept terms but don't enter phone
	err = it.Page.Locator("#terms").Click()
	it.requireNoError(err, "Failed to click terms checkbox")

	// Click submit
	err = it.Page.Locator("button[type=submit]:text('Continue')").Click()
	it.requireNoError(err, "Failed to click submit button")

	// Check if the phone input is marked as invalid by the browser
	isInvalid, err := it.Page.Locator("#phone").Evaluate(`(el) => el.validity.valueMissing`, nil)
	it.requireNoError(err, "Failed to check input validity")

	assert.True(t, isInvalid.(bool), "Phone input should be marked as invalid when empty")

	// Verify the form wasn't submitted by checking we're still on the same page
	currentURL := it.Page.URL()
	assert.Contains(t, currentURL, "/profile/resume/upload", "Should still be on the resume upload page")
}

// TestResumeUploadNoTerms tests the validation when terms are not accepted
func TestResumeUploadNoTerms(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping test in short mode")
	}

	it := NewIntegrationTest(t)
	defer it.Close()

	// Authenticate the test user
	it.Authenticate()

	it.NavigateTo("/profile/resume/upload")

	// Upload resume
	fileChooser, err := it.Page.ExpectFileChooser(func() error {
		return it.Page.Locator("input[type=file]").Click()
	})
	it.requireNoError(err, "Failed to get file chooser")

	err = fileChooser.SetFiles("../../testdata/sample-resumes/sample-resume.docx")
	it.requireNoError(err, "Failed to set file in file chooser")

	// Enter phone but don't accept terms
	err = it.Page.Locator("#phone").Fill("+15555555555")
	it.requireNoError(err, "Failed to enter phone number")

	// Set up error message listener
	errChan := make(chan string, 1)
	err = it.Page.ExposeFunction("testErrorCallback", func(args ...any) any {
		if len(args) > 0 {
			if msg, ok := args[0].(string); ok {
				errChan <- msg
			}
		}
		return nil
	})
	it.requireNoError(err, "Failed to expose error callback")

	// Add error listener
	_, err = it.Page.Evaluate(`
		window.addEventListener('htmx:beforeRequest', function(event) {
			event.detail.xhr.setRequestHeader('HX-Request', 'true');
		});

		window.addEventListener('htmx:responseError', function(event) {
			if (event.detail.error) {
				window.testErrorCallback(event.detail.error);
			}
		});
	`)
	it.requireNoError(err, "Failed to set up error listener")

	// Click submit
	err = it.Page.Locator("button[type=submit]:text('Continue')").Click()
	it.requireNoError(err, "Failed to click submit button")

	// Wait for error message
	select {
	case errorMsg := <-errChan:
		assert.Contains(t, errorMsg, "terms", "Error should mention terms of service")
	case <-time.After(5 * time.Second):
		t.Fatal("Timeout waiting for error message")
	}
}

// TestResumeUploadNoFile tests the validation when no resume is uploaded
func TestResumeUploadNoFile(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping test in short mode")
	}

	it := NewIntegrationTest(t)
	defer it.Close()

	// Authenticate the test user
	it.Authenticate()

	it.NavigateTo("/profile/resume/upload")

	// Enter phone and accept terms but don't upload resume
	err := it.Page.Locator("#phone").Fill("+15555555555")
	it.requireNoError(err, "Failed to enter phone number")

	err = it.Page.Locator("#terms").Click()
	it.requireNoError(err, "Failed to click terms checkbox")

	// Set up error message listener
	errChan := make(chan string, 1)
	err = it.Page.ExposeFunction("testErrorCallback", func(args ...any) any {
		if len(args) > 0 {
			if msg, ok := args[0].(string); ok {
				errChan <- msg
			}
		}
		return nil
	})
	it.requireNoError(err, "Failed to expose error callback")

	// Add error listener
	_, err = it.Page.Evaluate(`
		window.addEventListener('htmx:beforeRequest', function(event) {
			event.detail.xhr.setRequestHeader('HX-Request', 'true');
		});

		window.addEventListener('htmx:responseError', function(event) {
			if (event.detail.error) {
				window.testErrorCallback(event.detail.error);
			}
		});
	`)
	it.requireNoError(err, "Failed to set up error listener")

	// Click submit
	err = it.Page.Locator("button[type=submit]:text('Continue')").Click()
	it.requireNoError(err, "Failed to click submit button")

	// Wait for error message
	select {
	case errorMsg := <-errChan:
		assert.Contains(t, errorMsg, "resume", "Error should mention resume requirement")
	case <-time.After(5 * time.Second):
		t.Fatal("Timeout waiting for error message")
	}
}

// TestResumeUploadInvalidFileType tests uploading an invalid file type
func TestResumeUploadInvalidFileType(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping test in short mode")
	}

	it := NewIntegrationTest(t)
	defer it.Close()

	// Authenticate the test user
	it.Authenticate()

	it.NavigateTo("/profile/resume/upload")

	// Create and upload an invalid file
	invalidFilePath := "../../testdata/invalid.xyz"
	err := os.WriteFile(invalidFilePath, []byte("invalid content"), 0644)
	it.requireNoError(err, "Failed to create invalid file")
	defer os.Remove(invalidFilePath)

	// Upload invalid file
	fileChooser, err := it.Page.ExpectFileChooser(func() error {
		return it.Page.Locator("input[type=file]").Click()
	})
	it.requireNoError(err, "Failed to get file chooser")

	err = fileChooser.SetFiles(invalidFilePath)
	it.requireNoError(err, "Failed to set file in file chooser")

	// Enter phone and accept terms
	err = it.Page.Locator("#phone").Fill("+15555555555")
	it.requireNoError(err, "Failed to enter phone number")

	err = it.Page.Locator("#terms").Click()
	it.requireNoError(err, "Failed to click terms checkbox")

	// Set up error message listener
	errChan := make(chan string, 1)
	err = it.Page.ExposeFunction("testErrorCallback", func(args ...any) any {
		if len(args) > 0 {
			if msg, ok := args[0].(string); ok {
				errChan <- msg
			}
		}
		return nil
	})
	it.requireNoError(err, "Failed to expose error callback")

	// Add error listener
	_, err = it.Page.Evaluate(`
		window.addEventListener('htmx:beforeRequest', function(event) {
			event.detail.xhr.setRequestHeader('HX-Request', 'true');
		});

		window.addEventListener('htmx:responseError', function(event) {
			if (event.detail.error) {
				window.testErrorCallback(event.detail.error);
			}
		});
	`)
	it.requireNoError(err, "Failed to set up error listener")

	// Click submit
	err = it.Page.Locator("button[type=submit]:text('Continue')").Click()
	it.requireNoError(err, "Failed to click submit button")

	// Wait for error message
	select {
	case errorMsg := <-errChan:
		assert.Contains(t, errorMsg, "file type", "Error should mention invalid file type")
	case <-time.After(5 * time.Second):
		t.Fatal("Timeout waiting for error message")
	}
}
