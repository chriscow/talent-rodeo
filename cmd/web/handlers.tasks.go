package main

import (
	"net/http"
	"talent-rodeo/internal"
	"talent-rodeo/internal/response"
	"talent-rodeo/internal/tasks"

	"github.com/getsentry/sentry-go"
)

func (app *application) showTasksPage(w http.ResponseWriter, r *http.Request) {
	// Check if user is authenticated and is a super user
	user := contextGetAuthenticatedUser(r)
	if user == nil {
		http.Redirect(w, r, "/auth/linkedin", http.StatusSeeOther)
		return
	}
	if !user.IsSuperUser {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	// Create task storage
	taskStorage := tasks.NewFirestoreTaskStorage(app.fs)

	// Get status filter
	filter := &tasks.TaskFilter{}
	if status := r.URL.Query().Get("status"); status != "" {
		filter.Status = status
	}

	// Get all tasks from Firestore
	tasks, err := taskStorage.GetTasksWithFilter(r.Context(), filter)
	if err != nil {
		internal.SentryReportError(r.Context(), err, sentry.LevelError, map[string]string{
			"action": "get_tasks_with_filter_page",
			"userID": user.ID,
			"status": filter.Status,
		})
		app.serverError(w, r, err)
		return
	}

	data := app.newTemplateData(r)
	data["Tasks"] = tasks

	err = response.Page(w, http.StatusOK, data, "pages/admin/tasks.html")
	if err != nil {
		internal.SentryReportError(r.Context(), err, sentry.LevelError, map[string]string{
			"action":   "render_template",
			"template": "pages/admin/tasks.html",
			"userID":   user.ID,
		})
		app.serverError(w, r, err)
	}
}

func (app *application) getTasksPartial(w http.ResponseWriter, r *http.Request) {
	// Check if user is authenticated and is a super user
	user := contextGetAuthenticatedUser(r)
	if user == nil {
		http.Redirect(w, r, "/auth/linkedin", http.StatusSeeOther)
		return
	}
	if !user.IsSuperUser {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	// Create task storage
	taskStorage := tasks.NewFirestoreTaskStorage(app.fs)

	// Get status filter
	filter := &tasks.TaskFilter{}
	if status := r.URL.Query().Get("status"); status != "" {
		filter.Status = status
	}

	// Get all tasks from Firestore
	tasks, err := taskStorage.GetTasksWithFilter(r.Context(), filter)
	if err != nil {
		app.serverError(w, r, err)
		internal.SentryReportError(r.Context(), err, sentry.LevelError, map[string]string{
			"action": "get_tasks_with_filter_partial",
			"userID": user.ID,
			"status": filter.Status,
		})
		return
	}

	data := app.newTemplateData(r)
	data["Tasks"] = tasks

	err = response.Partial(w, http.StatusOK, data, "partials/tasks-table.html")
	if err != nil {
		app.serverError(w, r, err)
		internal.SentryReportError(r.Context(), err, sentry.LevelError, map[string]string{
			"action":   "render_partial",
			"template": "partials/tasks-table.html",
			"userID":   user.ID,
		})
	}
}
