package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"mime"
	"net/http"
	"os"
	"os/exec"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/bootdotdev/learn-file-storage-s3-golang-starter/internal/auth"
	"github.com/google/uuid"
)

func (cfg *apiConfig) handlerUploadVideo(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<30)
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
	key := initial_url + ".mp4"

	aspectRatio, err := getVideoAspectRatio(f.Name())
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "error in getting aspect ratio", err)
		return
	}

	if aspectRatio == "16:9"{
		key = "landscape/" + key
	}else if aspectRatio == "9:16"{
		key = "portrait/" + key
	}else {
		key = "other/" + key
	}

	bucket_name := "tubely-545556"
	params := &s3.PutObjectInput{
		Bucket: &bucket_name,
		Key: &key,
		Body: f,
		ContentType: &mediaType,
	}
	_, err = cfg.s3Client.PutObject(context.Background(), params)
	if err != nil{
		respondWithError(w, http.StatusInternalServerError, "error in uploading video of s3 object", err)
		return
	}

	region := os.Getenv("S3_REGION")
	videoURL := "https://" + bucket_name + ".s3." + region + ".amazonaws.com/" + key

	video.VideoURL = &videoURL
	err = cfg.db.UpdateVideo(video)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "error in updating video url", err)
		return
	}
	log.Println("Video Uploaded Successfully")
	respondWithJSON(w, http.StatusOK, video)
}


func getVideoAspectRatio(filePath string) (string, error){
	cmd := exec.Command("ffprobe", "-v", "error", "-print_format", "json", "-show_streams", filePath)
	var out bytes.Buffer
	cmd.Stdout = &out
	
	err := cmd.Run()
	if err != nil {
		return "", err
	}
	// buf := bytes.Buffer{}
	// buf.Write([]byte())
	aspectRatio := aspectRatio{}
	err = json.Unmarshal(out.Bytes(), &aspectRatio)
	if err != nil {
		return "", err
	}

	for _, stream := range aspectRatio.Streams{
		if stream.CodecType == "video"{
			// result := stream.AspectRatio
			ratio := float64(stream.Width) / float64(stream.Height)
			if ratio > 1.7 && ratio < 1.8{
				return "16:9", nil
			}
			if ratio > 0.5 && ratio < 0.6{
				return "9:16", nil
			}
			return "other", nil
		}
	} 
	return "", fmt.Errorf("No video found")
}

type aspectRatio struct {
	Streams 		[]Stream 	`json:"streams"`
}

type Stream struct{
	Width 		int  		`json:"width"`
	Height 		int 		`json:"height"`
	CodecType	string 		`json:"codec_type"`
	AspectRatio string 		`json:"display_aspect_ratio"`
}