package s3

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/url"

	"github.com/dgrr/rfs"

	"github.com/aws/aws-sdk-go-v2/aws"
	s3aws "github.com/aws/aws-sdk-go-v2/service/s3"
)

// File ...
type File struct {
	bucket   string
	path     string
	meta     map[string]interface{}
	size     int64
	cursor   int64
	uploadID string
	c        *s3aws.Client
}

func (f *File) Read(_ []byte) (int, error) {
	return -1, errors.New("file not open for reading")
}

func (f *File) Write(_ []byte) (int, error) {
	return -1, errors.New("file not open for writing")
}

// FileReader ...
type FileReader struct {
	File
}

// FileWriter ...
type FileWriter struct {
	File
	partNum int64
	cmpl    s3aws.CompletedMultipartUpload
	b       []byte
}

// NewWriter ...
func NewWriter(c *s3aws.Client) *FileWriter {
	size := 5 * 1024 * 1024 // min size is 5MB
	f := &FileWriter{
		partNum: 1,
		b:       make([]byte, size),
	}
	f.c = c
	f.meta = make(map[string]interface{})
	return f
}

// URL ...
func (f *File) URL() *url.URL {
	uri, err := url.Parse(
		fmt.Sprintf("%s://%s/%s", Kind, f.bucket, f.path),
	)
	if err == nil {
		return uri
	}
	return nil
}

// Stat ...
func (f *File) Stat() (rfs.Stat, error) {
	return f.meta, nil
}

// Read ...
func (f *FileReader) Read(b []byte) (int, error) {
	if f.c == nil {
		return -1, errors.New("file closed")
	}
	return 0, nil
}

// Write ...
func (f *FileWriter) Write(b []byte) (n int, err error) {
	if f.c == nil {
		return -1, errors.New("file closed")
	}

	n = copy(f.b[f.size:], b)
	f.size += int64(n)

	if f.size == int64(cap(f.b)) {
		r := len(b) - n // bytes to append
		err = f.Flush()
		if err == nil {
			f.b = append(f.b[:0], b[n:]...)
			f.size = int64(r)
			n += r // append bytes readed
		}
	}

	return n, nil
}

// Flush ...
func (f *FileWriter) Flush() error {
	size := len(f.b)
	partNum := f.partNum

	resp, err := f.c.UploadPartRequest(&s3aws.UploadPartInput{
		Bucket:        aws.String(f.bucket),
		Key:           aws.String(f.path),
		Body:          bytes.NewReader(f.b[:size]),
		ContentLength: aws.Int64(int64(size)),
		UploadId:      aws.String(f.uploadID),
		PartNumber:    aws.Int64(partNum),
	}).Send(context.Background())
	if err != nil {
		return fmt.Errorf("Flush(): %s", err)
	}
	f.cmpl.Parts = append(f.cmpl.Parts, s3aws.CompletedPart{
		ETag:       resp.ETag,
		PartNumber: &partNum,
	})

	f.size = 0
	f.partNum++

	return nil
}

// Close ...
func (f *FileReader) Close() error {
	if f.c == nil {
		return errors.New("file already closed")
	}

	f.c = nil

	return nil
}

// Close ...
func (f *FileWriter) Close() error {
	if f.c == nil {
		return errors.New("file already closed")
	}

	err := f.Flush()
	if err != nil {
		return err
	}
	resp, err := f.c.CompleteMultipartUploadRequest(&s3aws.CompleteMultipartUploadInput{
		Bucket:          aws.String(f.bucket),
		Key:             aws.String(f.path),
		UploadId:        aws.String(f.uploadID),
		MultipartUpload: &f.cmpl,
	}).Send(context.Background())
	if err != nil {
		return fmt.Errorf("Close(): %s", err)
	}
	f.c = nil
	f.bucket = aws.StringValue(resp.Bucket)
	f.path = aws.StringValue(resp.Key)
	f.meta[ETag] = aws.StringValue(resp.ETag)

	return nil
}