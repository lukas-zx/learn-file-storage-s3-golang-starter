package main

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"
	"log"
	"mime"
	"net/http"
	"os"
	"path/filepath"
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

	// "thumbnail" should match the HTML form input name
	// `file` is an `io.Reader` that we can read from to get the image data
	file, header, err := r.FormFile("thumbnail")
	if err != nil {
		log.Println(err)
		respondWithError(w, http.StatusBadRequest, "Unable to parse form file", err)
		return
	}
	defer file.Close()

	contentTypeHeader := header.Header.Get("Content-Type")
	mediaType, _, err := mime.ParseMediaType(contentTypeHeader)
	if err != nil {
		log.Println(err)
		respondWithError(w, http.StatusBadRequest, "Unable to get mediaType", err)
		return
	}
	if mediaType != "image/jpeg" && mediaType != "image/png" {
		log.Println(err)
		respondWithError(w, http.StatusBadRequest, "Unable to get mediaType", err)
		return
	}

	metadata, err := cfg.db.GetVideo(videoID)
	if err != nil {
		log.Println(err)
		respondWithError(w, http.StatusBadRequest, "Unable to get video metadata", err)
		return
	}
	if metadata.UserID != userID {
		respondWithError(w, http.StatusUnauthorized, "Not your video m8", nil)
		return
	}

	randomBytes := make([]byte, 32)
	if _, err = rand.Read(randomBytes); err != nil {
		log.Println(err)
		respondWithError(w, http.StatusInternalServerError, "Unable to create image name", err)
		return
	}

	fileName := base64.RawURLEncoding.EncodeToString(randomBytes)
	fileExtension := strings.Split(mediaType, "/")[1]
	fileName = fmt.Sprintf("%s.%s", fileName, fileExtension)
	filePath := filepath.Join(cfg.assetsRoot, fileName)

	newFile, err := os.Create(filePath) 
	if err != nil {
		log.Println(err)
		respondWithError(w, http.StatusUnauthorized, "Not your video m8", nil)
		return
	}

	io.Copy(newFile, file)

	thumbnailURL := fmt.Sprintf("http://localhost:%s/assets/%s", cfg.port, fileName)
	metadata.ThumbnailURL = &thumbnailURL

	if err = cfg.db.UpdateVideo(metadata); err != nil {
		log.Println(err)
		respondWithError(w, http.StatusInternalServerError, "Unable to update video", err)
		return
	}

	respondWithJSON(w, http.StatusOK, metadata)
}
