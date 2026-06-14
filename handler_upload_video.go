package main

import (
	"context"
	"io"
	"mime"
	"net/http"
	"os"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/bootdotdev/learn-file-storage-s3-golang-starter/internal/auth"
	"github.com/google/uuid"
)

func (cfg *apiConfig) handlerUploadVideo(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 30)
	videoIDStr := r.PathValue("videoID")
	videoID, err := uuid.Parse(videoIDStr)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Unable to Parse Video Id", err)
		return
	}

	token, err := auth.GetBearerToken(r.Header)
	if err != nil{
		respondWithError(w, http.StatusUnauthorized, "bearer token not found", err)
		return
	}

	userId, err := auth.ValidateJWT(token, cfg.jwtSecret)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "user not found", err)
		return
	}

	video, err := cfg.db.GetVideo(videoID)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "error in getting video", err)
		return
	}
	if video.UserID != userId{
		respondWithError(w, http.StatusUnauthorized, "Not a authorized user", err)
		return
	}

	multi_partFile, multipart_header, err := r.FormFile("video")
	if err != nil{
		respondWithError(w, http.StatusInternalServerError, "Unable to form multi part file", err)
		return
	}
	defer multi_partFile.Close()

	content_type := multipart_header.Header.Get("Content-Type")
	mediaType, _, err := mime.ParseMediaType(content_type)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "the uploaded media type is not valid", err)
		return
	}
	if mediaType != "video/mp4"{
		respondWithError(w, http.StatusBadRequest, "Not a valid media type", err)
		return 
	}

	f, err := os.CreateTemp("", "tubely-upload.mp4")
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "error in creating temperory file", err)
		return
	}
	defer os.Remove(f.Name())
	defer f.Close()
	_, err = io.Copy(f, multi_partFile)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Error in Writing in temp file", err)
		return
	}
	_, err = f.Seek(0, io.SeekStart)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Error in setting offset to zero", err)
		return
	}

	initial_url := randomBase64string()
	url := initial_url + ".mp4"

	bucket_name := "tubely-545556"
	params := &s3.PutObjectInput{
		Bucket: &bucket_name,
		Key: &url,
		Body: f,
		ContentType: &mediaType,
	}
	_, err = cfg.s3Client.PutObject(context.Background(), params)
	if err != nil{
		respondWithError(w, http.StatusInternalServerError, "error in uploading video of s3 object", err)
		return
	}

	region := os.Getenv("S3_REGION")
	videoURL := "https://" + bucket_name + ".s3." + region + ".amazonaws.com/" + url

	video.VideoURL = &videoURL
	err = cfg.db.UpdateVideo(video)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "error in updating video url", err)
		return
	}
	respondWithJSON(w, http.StatusOK, video)
}