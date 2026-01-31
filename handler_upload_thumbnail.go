package main

import (
	"encoding/base64"
	"fmt"
	"io"
	"log"
	"net/http"

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


	fmt.Println("uploading thumbnail for video", videoID, "by user", userID)

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

	mediaType := header.Header.Get("Content-Type")
	imageData, err := io.ReadAll(file)
	if err != nil {
		log.Println(err)
		respondWithError(w, http.StatusBadRequest, "Unable to parse form file", err)
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
	}

	imageBase64 := base64.StdEncoding.EncodeToString(imageData)
	dataURL := fmt.Sprintf("data:%s;base64,%s", mediaType, imageBase64)
	metadata.ThumbnailURL = &dataURL

	if err = cfg.db.UpdateVideo(metadata); err != nil {
		log.Println(err)
		respondWithError(w, http.StatusInternalServerError, "Unable to update video", err)
		return
	}

	respondWithJSON(w, http.StatusOK, metadata)
}
