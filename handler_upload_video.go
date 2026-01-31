package main

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"mime"
	"net/http"
	"os"
	"os/exec"
	"strings"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/bootdotdev/learn-file-storage-s3-golang-starter/internal/auth"
	"github.com/google/uuid"
)

func getVideoAspectRatio(filePath string) (string, error) {
	cmd := exec.Command(
		"ffprobe",
		"-v", "error",
		"-print_format", "json",
		"-show_streams",
		filePath,
	)

	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Run()

	var data struct {
		Streams []struct {
			CodecType string `json:"codec_type"`
			Width     int    `json:"width"`
			Height    int    `json:"height"`
		} `json:"streams"`
	}

	json.Unmarshal(out.Bytes(), &data)

	for _, s := range data.Streams {
		if s.CodecType == "video" && s.Width > 0 && s.Height > 0 {

			ratio := (s.Width * 1000) / s.Height // INT division

			switch {
			case ratio >= 1700 && ratio <= 1850:
				return "16:9", nil
			case ratio >= 520 && ratio <= 600:
				return "9:16", nil
			default:
				return "other", nil
			}
		}
	}

	return "other", nil
}

func (cfg *apiConfig) handlerUploadVideo(w http.ResponseWriter, r *http.Request) {
	// parameter parsing
	videoIDString := r.PathValue("videoID")
	videoID, err := uuid.Parse(videoIDString)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid ID", err)
		return
	}

	// auth
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

	// ensure request comes from the video owner
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

	const maxMemory = 1 << 30
	r.Body = http.MaxBytesReader(w, r.Body, maxMemory)
	if err := r.ParseMultipartForm(maxMemory); err != nil {
		respondWithError(w, http.StatusBadRequest, "Upload too large or invalid multipart form", err)
		return
	}

	// "video" should match the HTML form input name
	// `file` is an `io.Reader` that we can read from to get the video data
	file, header, err := r.FormFile("video")
	if err != nil {
		log.Println(err)
		respondWithError(w, http.StatusBadRequest, "Unable to parse form file", err)
		return
	}
	defer file.Close()

	// ensure correct mime type
	contentTypeHeader := header.Header.Get("Content-Type")
	mediaType, _, err := mime.ParseMediaType(contentTypeHeader)
	if err != nil {
		log.Println(err)
		respondWithError(w, http.StatusBadRequest, "Unable to get mediaType", err)
		return
	}
	if mediaType != "video/mp4" {
		log.Println(err)
		respondWithError(w, http.StatusBadRequest, "Unable to get mediaType", err)
		return
	}

	tempFile, err := os.CreateTemp("", "tubely-upload.mp4")
	if err != nil {
		log.Println(err)
		respondWithError(w, http.StatusInternalServerError, "Unable to create temp file", err)
		return
	}
	defer os.Remove(tempFile.Name())
	defer tempFile.Close() // defer = LIFO, so close needs to be used second

	if _, err = io.Copy(tempFile, file); err != nil {
		log.Println(err)
		respondWithError(w, http.StatusInternalServerError, "Unable to copy to temp file", nil)
		return
	}
	tempFile.Seek(0, io.SeekStart)

	// random video name
	randomBytes := make([]byte, 32)
	if _, err = rand.Read(randomBytes); err != nil {
		log.Println(err)
		respondWithError(w, http.StatusInternalServerError, "Unable to create image name", err)
		return
	}

	fileName := base64.RawURLEncoding.EncodeToString(randomBytes)
	fileExtension := strings.Split(mediaType, "/")[1]
	fileName = fmt.Sprintf("%s.%s", fileName, fileExtension)

	aspectRatio, err := getVideoAspectRatio(tempFile.Name())
	if err != nil {
		log.Println(err)
		respondWithError(w, http.StatusInternalServerError, "Unable to get file aspect ratio", err)
		return
	}

	fmt.Println("aspectRatio")
	fmt.Println(aspectRatio)

	switch aspectRatio {
	case "16:9":
		fileName = fmt.Sprintf("landscape/%s", fileName)
	case "9:16":
		fileName = fmt.Sprintf("portrait/%s", fileName)
	default:
		fileName = fmt.Sprintf("other/%s", fileName)
	}

	cfg.s3Client.PutObject(r.Context(), &s3.PutObjectInput{
		Bucket:      &cfg.s3Bucket,
		Key:         &fileName,
		Body:        tempFile,
		ContentType: &mediaType,
	})

	videoURL := fmt.Sprintf("https://%s.s3.%s.amazonaws.com/%s", cfg.s3Bucket, cfg.s3Region, fileName)
	metadata.VideoURL = &videoURL

	if err = cfg.db.UpdateVideo(metadata); err != nil {
		log.Println(err)
		respondWithError(w, http.StatusInternalServerError, "Unable to update video", err)
		return
	}

	respondWithJSON(w, http.StatusOK, metadata)
}
