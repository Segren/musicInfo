package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/Segren/testTask/internal/data"
	"github.com/Segren/testTask/internal/validator"
	"net/http"
	"net/url"
)

// @Summary Get list of songs
// @Description Retrieve a list of songs with optional filters and pagination
// @Tags songs
// @Accept json
// @Produce json
// @Param group query string false "Filter by group"
// @Param name query string false "Filter by song name"
// @Param page query int false "Page number"
// @Param page_size query int false "Number of items per page"
// @Param sort query string false "Sort order (e.g., 'id', '-id', 'name', '-name')"
// @Success 200 {object} map[string]interface{} "List of songs with metadata"
// @Failure 400 {object} map[string]string "Bad request"
// @Failure 500 {object} map[string]string "Internal server error"
// @Router /songs [get]
func (app *application) listSongsHandler(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Name  string
		Group string
		data.Filters
	}

	v := validator.New()

	qs := r.URL.Query()

	input.Group = app.readString(qs, "group", "")
	input.Name = app.readString(qs, "name", "")

	input.Filters.Page = app.readInt(qs, "page", 1, v)
	input.Filters.PageSize = app.readInt(qs, "page_size", 20, v)
	input.Filters.Sort = app.readString(qs, "sort", "id")

	input.Filters.SortSafelist = []string{"id", "group", "name", "-id", "-group", "-name"}

	songs, metadata, err := app.models.Songs.GetAll(input.Name, input.Group, input.Filters)
	if err != nil {
		app.serverErrorResponse(w, r, err)
		return
	}

	err = app.writeJSON(w, http.StatusOK, envelope{"songs": songs, "metadata": metadata}, nil)
	if err != nil {
		app.serverErrorResponse(w, r, err)
	}
}

// @Summary Add a new song
// @Description Create a new song by providing the group name and song title. Additional details are fetched from an external API.
// @Tags songs
// @Accept json
// @Produce json
// @Param song body data.Song true "Group and song details"
// @Success 201 {object} data.Song "The newly created song"
// @Header 201 {string} Location "/songs/{id}" "URL of the created song"
// @Failure 400 {object} map[string]string "Invalid request data"
// @Failure 422 {object} map[string]interface{} "Validation error"
// @Failure 500 {object} map[string]string "Internal server error"
// @Router /songs [post]
func (app *application) createSongHandler(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Group string `json:"group"`
		Song  string `json:"song"`
	}

	err := app.readJSON(w, r, &input)
	if err != nil {
		app.badRequestResponse(w, r, err)
		return
	}

	// Запрос к внешнему API для получения дополнительных данных.
	songDetail, err := app.fetchSongDetails(input.Group, input.Song)
	if err != nil {
		app.serverErrorResponse(w, r, err)
		return
	}

	song := &data.Song{
		Group:       input.Group,
		Song:        input.Song,
		ReleaseDate: songDetail.ReleaseDate,
		Text:        songDetail.Text,
		Link:        songDetail.Link,
	}

	//инициализация валидатора
	v := validator.New()

	if data.ValidateSong(v, song); !v.Valid() {
		app.failedValidationResponse(w, r, v.Errors)
		return
	}

	err = app.models.Songs.Insert(song)
	if err != nil {
		app.serverErrorResponse(w, r, err)
		return
	}

	headers := make(http.Header)
	headers.Set("Location", fmt.Sprintf("/song/%d", song.ID))

	err = app.writeJSON(w, http.StatusCreated, envelope{"song": song}, headers)
	if err != nil {
		app.serverErrorResponse(w, r, err)
	}
}

func (app *application) fetchSongDetails(group, song string) (*data.Song, error) {
	//тут должен быть нужный внешний адрес
	url := fmt.Sprintf("http://localhost:8081/info?group=%s&song=%s", url.QueryEscape(group), url.QueryEscape(song))

	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	var songDetail struct {
		ReleaseDate string `json:"releaseDate"`
		Text        string `json:"text"`
		Link        string `json:"link"`
	}

	err = json.NewDecoder(resp.Body).Decode(&songDetail)
	if err != nil {
		return nil, err
	}

	return &data.Song{
		ReleaseDate: songDetail.ReleaseDate,
		Text:        songDetail.Text,
		Link:        songDetail.Link,
	}, nil
}

// @Summary Delete a song
// @Description Delete a song by its ID
// @Tags songs
// @Accept json
// @Produce json
// @Param id path int true "Song ID"
// @Success 200 {object} map[string]string "Message indicating successful deletion"
// @Failure 404 {object} map[string]string "Song not found"
// @Failure 500 {object} map[string]string "Internal server error"
// @Router /songs/{id} [delete]
func (app *application) deleteSongHandler(w http.ResponseWriter, r *http.Request) {
	id, err := app.readIDParam(r)
	if err != nil {
		app.notFoundResponse(w, r)
		return
	}

	err = app.models.Songs.Delete(id)
	if err != nil {
		switch {
		case errors.Is(err, data.ErrRecordNotFound):
			app.notFoundResponse(w, r)
		default:
			app.serverErrorResponse(w, r, err)
		}
		return
	}

	err = app.writeJSON(w, http.StatusOK, envelope{"message": "song successfully deleted"}, nil)
	if err != nil {
		app.serverErrorResponse(w, r, err)
	}
}

// @Summary Update song details
// @Description Update the details of an existing song by its ID.
// @Tags songs
// @Accept json
// @Produce json
// @Param id path int true "Song ID"
// @Param input body map[string]interface{} true "Song details to update"
// @Success 200 {object} data.Song "Updated song data"
// @Failure 400 {object} map[string]string "Bad request or invalid input"
// @Failure 404 {object} map[string]string "Song not found"
// @Failure 409 {object} map[string]string "Edit conflict occurred"
// @Failure 422 {object} map[string]interface{} "Validation error"
// @Failure 500 {object} map[string]string "Internal server error"
// @Router /songs/{id} [put]
func (app *application) updateSongHandler(w http.ResponseWriter, r *http.Request) {
	id, err := app.readIDParam(r)
	if err != nil {
		app.notFoundResponse(w, r)
		return
	}

	song, err := app.models.Songs.Get(id)
	if err != nil {
		switch {
		case errors.Is(err, data.ErrRecordNotFound):
			app.notFoundResponse(w, r)
		default:
			app.serverErrorResponse(w, r, err)
		}
		return
	}

	var input struct {
		Group *string `json:"group"`
		Song  *string `json:"song"`
	}

	err = app.readJSON(w, r, &input)
	if err != nil {
		app.badRequestResponse(w, r, err)
		return
	}

	if input.Group != nil {
		song.Group = *input.Group
	}
	if input.Song != nil {
		song.Song = *input.Song
	}

	v := validator.New()

	if data.ValidateSong(v, song); !v.Valid() {
		app.failedValidationResponse(w, r, v.Errors)
		return
	}

	err = app.models.Songs.Update(song)
	if err != nil {
		switch {
		case errors.Is(err, data.ErrEditConflict):
			app.editConflictResponse(w, r)
		default:
			app.serverErrorResponse(w, r, err)
		}
		return
	}

	err = app.writeJSON(w, http.StatusOK, envelope{"song": song}, nil)
	if err != nil {
		app.serverErrorResponse(w, r, err)
	}
}

// @Summary Get lyrics of a song
// @Description Retrieve song lyrics with pagination by verses
// @Tags songs
// @Accept json
// @Produce json
// @Param id path int true "Song ID"
// @Param page query int false "Page number"
// @Param size query int false "Number of verses per page"
// @Success 200 {object} map[string]interface{} "Lyrics with pagination"
// @Failure 400 {object} map[string]string "Bad request or invalid parameters"
// @Failure 404 {object} map[string]string "Song not found"
// @Failure 500 {object} map[string]string "Internal server error"
// @Router /songs/{id}/lyrics [get]
func (app *application) getSongLyricsHandler(w http.ResponseWriter, r *http.Request) {
	id, err := app.readIDParam(r)
	if err != nil {
		app.notFoundResponse(w, r)
		return
	}

	song, err := app.models.Songs.Get(id)
	if err != nil {
		if errors.Is(err, data.ErrRecordNotFound) {
			app.notFoundResponse(w, r)
			return
		}
		app.serverErrorResponse(w, r, err)
		return
	}

	v := validator.New()

	if data.ValidateSong(v, song); !v.Valid() {
		app.failedValidationResponse(w, r, v.Errors)
		return
	}

	page := app.readInt(r.URL.Query(), "page", 1, v)
	size := app.readInt(r.URL.Query(), "size", 1, v)

	lyrics, err := app.models.Songs.GetLyricsByID(song, id, page, size)
	if err != nil {
		app.serverErrorResponse(w, r, err)
		return
	}

	app.writeJSON(w, http.StatusOK, envelope{"lyrics": lyrics}, nil)
}
