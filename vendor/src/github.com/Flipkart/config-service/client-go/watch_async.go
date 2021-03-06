package cfgsvc
import (
	"log"
	"net/http"
	"errors"
	"github.com/pquerna/ffjson/ffjson"
)

const (
	INTERNAL_ERROR = 500
	BUCKET_NOT_FOUND = 404
	BUCKET_NOT_MODIFIED = 304
)
type BucketResponse struct {
	bucket *Bucket
	statusCode int
	err error
}

type ErrorResp struct{
	ErrorType string `json:"type"`
	Message string `json:"message"`
}

func (this *ErrorResp) Error() string{
	return this.Message
}

type WatchAsync struct {
	bucketName string
	httpClient *HttpClient
	dynamicBucket *DynamicBucket
	asyncResp chan *BucketResponse
}

func (this *WatchAsync) watch() (<-chan *BucketResponse) {
	go func() {
		resp, err := this.httpClient.get(this.bucketName, LATEST_VERSION, true, this.dynamicBucket.GetVersionAsString())
		if err != nil {
			log.Println("Error making request", err)
			this.asyncResp <- &BucketResponse{bucket: nil, err: err, statusCode: INTERNAL_ERROR}
		} else {
			defer resp.Body.Close()
			this.handleResp(resp)
		}
	}()
	return this.asyncResp
}

func (this *WatchAsync) handleResp(resp *http.Response) {
	if (isBucketDeleted(resp)) {
		this.asyncResp <- &BucketResponse{bucket: nil, err: errors.New("Bucket is deleted"), statusCode: resp.StatusCode}
	} else {
		this.createNewBucket(resp)
	}
}

func (this *WatchAsync) createNewBucket(resp *http.Response) {
	httpClient := this.httpClient
	asyncResp := this.asyncResp

	this.dynamicBucket.Connected()

	data, err := httpClient.readResp(resp)
	if err != nil {
		log.Println("Error reading resp ", err.Error())
		asyncResp <- &BucketResponse{bucket: nil, err: err, statusCode: resp.StatusCode}
		return
	}

	newBucket, err := httpClient.newBucket(data)
	if err != nil {
		log.Println("Error while fetching bucket ", err)
		asyncResp <- &BucketResponse{bucket: nil, err: err, statusCode: resp.StatusCode}
	} else {
		if (newBucket.isValid()) {
			asyncResp <- &BucketResponse{bucket: newBucket, err: err, statusCode: resp.StatusCode}
		} else {
			this.handleInvalidBucket(data)
		}
	}

}

func (this *WatchAsync) handleInvalidBucket(data []byte) {
	asyncResp := this.asyncResp

	errResp := &ErrorResp{}
	err := ffjson.Unmarshal(data, errResp)
	if err != nil {
		asyncResp <- &BucketResponse{bucket: nil, err: errors.New("Error decoding to JSON"), statusCode: INTERNAL_ERROR}
	} else {
		if errResp.ErrorType == DELETED {
			asyncResp <- &BucketResponse{bucket: nil, err: errResp, statusCode: BUCKET_NOT_FOUND}
		} else if errResp.ErrorType == NOT_MODIFIED {
				asyncResp <- &BucketResponse{bucket: nil, err: errResp, statusCode: BUCKET_NOT_MODIFIED}
		} else {
			asyncResp <- &BucketResponse{bucket: nil, err: errResp, statusCode: INTERNAL_ERROR}
		}
	}
}

func isBucketDeleted(resp *http.Response) bool {
	errResp := &ErrorResp{}
	if resp.StatusCode == 404 {
		err := ffjson.NewDecoder().DecodeReader(resp.Body, errResp)
		if err != nil {
			log.Println("Error reading data", err)
		}
		log.Println("Error", errResp)
		return true
	}
	return false
}

