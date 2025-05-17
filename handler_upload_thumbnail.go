package main

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/bootdotdev/learn-file-storage-s3-golang-starter/internal/auth"
	"github.com/google/uuid"
)

func (cfg *apiConfig) handlerUploadThumbnail(w http.ResponseWriter, r *http.Request) {
	videoIDString := r.PathValue("videoID")
	videoID, err := uuid.Parse(videoIDString)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid ID", err)
		return
	}

	token, err := auth.GetBearerToken(r.Header)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "Couldn't find JWT", err)
		return
	}

	userID, err := auth.ValidateJWT(token, cfg.jwtSecret)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "Couldn't validate JWT", err)
		return
	}

	const maxMemory = 10 << 20
	r.ParseMultipartForm(maxMemory)
	file, header, err := r.FormFile("thumbnail")
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Error parsing", err)
	}
	defer file.Close()

	mediaType, _, err := mime.ParseMediaType(header.Header.Get("Content-Type"))
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Error parsing media type", err)
		return
	}

	supportedMimeTypes := [2]string{"image/jpeg", "image/png"}
	if !slices.Contains(supportedMimeTypes[:], mediaType) {
		respondWithError(w, http.StatusBadRequest, "unsupported media type", err)
		return
	}

	fileExt := strings.Split(mediaType, "/")

	somethingRandomData := make([]byte, 32)
	rand.Read(somethingRandomData)
	someRandomString := base64.RawURLEncoding.EncodeToString(somethingRandomData)
	fileName := fmt.Sprintf("%s.%s", someRandomString, fileExt[1])

	meta, err := cfg.db.GetVideo(videoID)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Error getting meta data from db", err)
		return
	}

	if meta.UserID != userID {
		respondWithError(w, http.StatusUnauthorized, "Not Authorized", err)
		return
	}

	path := filepath.Join(cfg.assetsRoot, fileName)

	newFile, err := os.Create(path)
	n, err := io.Copy(newFile, file)
	fmt.Println("bytes written to local disk:", n)
	url := fmt.Sprintf("http://localhost:%s/assets/%s", cfg.port, fileName)
	meta.ThumbnailURL = &url

	err = cfg.db.UpdateVideo(meta)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Error updating meta", err)
		return
	}

	respondWithJSON(w, http.StatusOK, meta)
}
