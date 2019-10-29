package s3

// S3Option is an option to the s3 backend
type s3Option interface {
	isS3Opt()
}

type serverSideEncryptionOpt ServerSideEncryptionType

func (s serverSideEncryptionOpt) isS3Opt() {}

// DeferFieldDecoding instructs the iterator to wait until fields are requested
// before decoding.
func S3ServerSideEncoding(sse ServerSideEncryptionType) serverSideEncryptionOpt {
	return serverSideEncryptionOpt(sse)
}
