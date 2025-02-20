package main

import (
	"net/http"
	"strconv"
	"talent-rodeo/internal/response"
)

func (app *application) showTasksPage(w http.ResponseWriter, r *http.Request) {
	// Check if user is authenticated and is a super user
	user := contextGetAuthenticatedUser(r)
	if user == nil {
		http.Redirect(w, r, "/auth/linkedin", http.StatusFound)
		return
	}
	if !user.IsSuperUser {
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}

	// Get query parameters
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}
	size, _ := strconv.Atoi(r.URL.Query().Get("size"))
	if size < 1 {
		size = 10
	}

	// Get status filter
	var statuses []string
	if status := r.URL.Query().Get("status"); status != "" {
		statuses = append(statuses, status)
	}

	// Get tasks from database
	tasks, err := app.db.GetTasksWithFilter(r.Context(), statuses, size, (page-1)*size)
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	// Get total count for pagination
	total, err := app.db.GetTasksCount(r.Context(), statuses)
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	data := app.newTemplateData(r)
	data["Tasks"] = tasks
	data["Pagination"] = map[string]int{
		"Page":  page,
		"Size":  size,
		"Total": total,
		"Pages": (total + size - 1) / size,
	}

	err = response.Page(w, http.StatusOK, data, "pages/admin/tasks.html")
	if err != nil {
		app.serverError(w, r, err)
	}
}

func (app *application) getTasksPartial(w http.ResponseWriter, r *http.Request) {
	// Check if user is authenticated and is a super user
	user := contextGetAuthenticatedUser(r)
	if user == nil {
		http.Redirect(w, r, "/auth/linkedin", http.StatusFound)
		return
	}
	if !user.IsSuperUser {
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}

	// Get query parameters
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}
	size, _ := strconv.Atoi(r.URL.Query().Get("size"))
	if size < 1 {
		size = 10
	}

	// Get status filter
	var statuses []string
	if status := r.URL.Query().Get("status"); status != "" {
		statuses = append(statuses, status)
	}

	// Get tasks from database
	tasks, err := app.db.GetTasksWithFilter(r.Context(), statuses, size, (page-1)*size)
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	// Get total count for pagination
	total, err := app.db.GetTasksCount(r.Context(), statuses)
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	data := app.newTemplateData(r)
	data["Tasks"] = tasks
	data["Pagination"] = map[string]int{
		"Page":  page,
		"Size":  size,
		"Total": total,
		"Pages": (total + size - 1) / size,
	}

	err = response.Partial(w, http.StatusOK, data, "partials/tasks-table.html")
	if err != nil {
		app.serverError(w, r, err)
	}
}
