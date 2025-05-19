package main

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"os/exec"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/bootdotdev/learn-file-storage-s3-golang-starter/internal/auth"
	"github.com/google/uuid"
)

func processVideoForFastStart(filePath string) (string, error) {
	fmt.Println("filePath", filePath)
	outputPath := filePath + ".processing"
	cmd := exec.Command("ffmpeg", "-i", filePath, "-c", "copy", "-movflags", "faststart", "-f", "mp4", outputPath)
	err := cmd.Run()
	if err != nil {
		return "", err
	}
	return outputPath, nil
}

func getVideoAspectRatio(filePath string) (string, error) {
	cmd := exec.Command("ffprobe", "-v", "error", "-print_format", "json", "-show_streams", filePath)
	var buff []byte
	buffer := bytes.NewBuffer(buff)
	cmd.Stdout = buffer
	err := cmd.Run()
	if err != nil {
		return "", err
	}

	type FFProbeOutput struct {
		Streams []struct {
			Width  int `json:"width"`
			Height int `json:"height"`
		} `json:"streams"`
	}

	var probeOut FFProbeOutput

	if err := json.Unmarshal(buffer.Bytes(), &probeOut); err != nil {
		return "", err
	}
	if len(probeOut.Streams) < 1 {
		return "", fmt.Errorf("no streams found")
	}

	height := probeOut.Streams[0].Height
	width := probeOut.Streams[0].Width
	// very lazy here to det aspect ratio. For the project and samples this is good 'nuff
	if width/height == 1 {
		return "16:9", nil
	}

	if height/width == 1 {
		return "9:16", nil
	}
	return "other", nil

}

func (cfg *apiConfig) handlerUploadVideo(w http.ResponseWriter, r *http.Request) {

	// limit upload size to 1gb
	r.Body = http.MaxBytesReader(w, r.Body, 1<<30)

	videoIDString := r.PathValue("videoID")
	videoID, err := uuid.Parse(videoIDString)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid Video ID", err)
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

	video, err := cfg.db.GetVideo(videoID)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Error getting meta data from db", err)
		return
	}

	if video.UserID != userID {
		respondWithError(w, http.StatusUnauthorized, "Not Authorized", err)
		return
	}

	file, header, err := r.FormFile("video")
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "video read error", err)
		return
	}
	defer file.Close()

	mediaType, _, err := mime.ParseMediaType(header.Header.Get("Content-Type"))
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Error parsing media type", err)
		return
	}

	if mediaType != "video/mp4" {
		respondWithError(w, http.StatusBadRequest, "unsupported media type", err)
		return
	}
	tempFileName := "temp_tubley_upload.mp4"
	tempFile, err := os.CreateTemp("", tempFileName)
	defer os.Remove(tempFileName)
	defer tempFile.Close()

	n, err := io.Copy(tempFile, file)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Copy error", err)
		return
	}
	// create the fast start video
	fastStartFilePath, err := processVideoForFastStart(tempFile.Name())
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "error processing video", err)
		return
	}

	// get a file reference for an io reader to that file
	fastStartFile, err := os.Open(fastStartFilePath)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "error processing file", err)
		return
	}

	defer os.Remove(fastStartFilePath)
	defer fastStartFile.Close()

	fmt.Println("bytes written to temp file:", n)
	aspectRatio, err := getVideoAspectRatio(tempFile.Name())

	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "aspect ratio error", err)
		return
	}

	prefix := "other"
	if aspectRatio == "16:9" {
		prefix = "landscape"
	}

	if aspectRatio == "9:16" {
		prefix = "portrait"
	}

	// reset the temp files pointer to the begining so we can read it again
	//tempFile.Seek(0, io.SeekStart)

	// make the file name
	randomPart := make([]byte, 32)
	rand.Read(randomPart)
	fileName := fmt.Sprintf("%s/%s.mp4", prefix, base64.RawURLEncoding.EncodeToString(randomPart))

	params := s3.PutObjectInput{
		Bucket:      &cfg.s3Bucket,
		Key:         &fileName,
		Body:        fastStartFile,
		ContentType: &mediaType,
	}

	_, err = cfg.s3Client.PutObject(r.Context(), &params)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "error uploading to s3", err)
		return
	}

	videoURL := fmt.Sprintf("https://%s.s3.%s.amazonaws.com/%s", cfg.s3Bucket, cfg.s3Region, fileName)
	video.VideoURL = &videoURL

	err = cfg.db.UpdateVideo(video)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "error updating db", err)
		return
	}

	respondWithJSON(w, http.StatusOK, video)
}
