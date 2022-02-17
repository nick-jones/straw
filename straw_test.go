package straw_test

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/nick-jones/straw"
	"github.com/pkg/sftp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/ssh"

	_ "github.com/nick-jones/straw/gcs"
	_ "github.com/nick-jones/straw/s3"
	_ "github.com/nick-jones/straw/sftp"
)

type fsTester struct {
	name     string
	fs       straw.StreamStore
	ff       func() straw.StreamStore
	testRoot string
}

func (fst *fsTester) TestOpenReadCloserNotExisting(t *testing.T) {
	assert := assert.New(t)

	f, err := fst.fs.OpenReadCloser("/does/not/exist")
	assert.True(os.IsNotExist(err))
	assert.Nil(f)
}

func (fst *fsTester) TestOpenReadCloserOnDirectory(t *testing.T) {
	assert := assert.New(t)
	require := require.New(t)

	name := filepath.Join(fst.testRoot, "TestOpenReadCloserOnDirectory")

	err := fst.fs.Mkdir(name, 0755)
	require.NoError(err)

	f, err := fst.fs.OpenReadCloser(name)
	assert.EqualError(err, fmt.Sprintf("%s is a directory", name))
	//	assert.Condition(func() bool { return strings.HasSuffix(err.Error(), "no such file or directory") })
	assert.Nil(f)
}

func (fst *fsTester) TestCreateNewWriteOnly(t *testing.T) {
	assert := assert.New(t)
	require := require.New(t)

	name := filepath.Join(fst.testRoot, "TestCreateNewWriteOnly")

	f, err := fst.fs.CreateWriteCloser(name)
	require.NoError(err)
	assert.NotNil(f)
	require.NoError(writeAll(f, []byte{0, 1, 2, 3, 4}))
	require.NoError(f.Close())

	fi, err := fst.fs.Stat(name)
	require.NoError(err)
	assert.Equal(fi.Size(), int64(5))
	assert.Equal(fi.IsDir(), false)

	files, err := fst.fs.Readdir(fst.testRoot)
	require.NoError(err)
	assert.Equal(1, len(files))

	assert.False(files[0].IsDir())
	assert.Equal("TestCreateNewWriteOnly", files[0].Name())
	assert.Equal(int64(5), files[0].Size())
	assert.Equal(os.FileMode(0644), files[0].Mode())
}

func (fst *fsTester) TestCreateWriteOnlyOnExistingDir(t *testing.T) {
	assert := assert.New(t)
	require := require.New(t)

	name := filepath.Join(fst.testRoot, "TestCreateWriteOnlyOnExistingDir")

	err := fst.fs.Mkdir(name, 0755)
	require.NoError(err)

	f, err := fst.fs.CreateWriteCloser(name)
	require.NotNil(err)
	assert.Condition(func() bool { return strings.HasSuffix(err.Error(), "is a directory") })
	assert.Nil(f)

	fi, err := fst.fs.Stat(name)
	require.NoError(err)
	assert.Equal(fi.Size(), int64(4096))
	assert.Equal(fi.IsDir(), true)
}

func (fst *fsTester) TestCreateWriteOnlyInExistingFile(t *testing.T) {
	assert := assert.New(t)
	require := require.New(t)

	filename := filepath.Join(fst.testRoot, "TestCreateWriteOnlyInExistingFile")
	f, err := fst.fs.CreateWriteCloser(filename)
	require.NoError(err)
	require.NoError(writeAll(f, []byte{0, 1, 2, 3, 4}))
	require.NoError(f.Close())

	name := filepath.Join(filename, "another_filename")

	f, err = fst.fs.CreateWriteCloser(name)
	require.NotNil(err)
	assert.Condition(func() bool { return strings.HasSuffix(err.Error(), "not a directory") }, "error does not match : %s", err.Error())
	assert.Nil(f)
}

func (fst *fsTester) TestMkdirAtRoot(t *testing.T) {
	assert := assert.New(t)
	require := require.New(t)

	name := filepath.Join(fst.testRoot, "TestMkdirAtRoot")

	err := fst.fs.Mkdir(name, 0755)
	require.NoError(err)

	fi, err := fst.fs.Stat(name)
	require.NoError(err)
	assert.Equal(fi.Size(), int64(4096))
	assert.Equal(fi.IsDir(), true)
}

func (fst *fsTester) TestMkdirTrailingSlash(t *testing.T) {
	assert := assert.New(t)
	require := require.New(t)

	name := filepath.Join(fst.testRoot, "TestMkdirTrailingSlash")
	name = name + "/"

	err := fst.fs.Mkdir(name, 0755)
	require.NoError(err)

	fi, err := fst.fs.Stat(name)
	require.NoError(err)
	assert.Equal(fi.Size(), int64(4096))
	assert.Equal(fi.IsDir(), true)
}

func (fst *fsTester) TestMkdirOnExistingDir(t *testing.T) {
	assert := assert.New(t)
	require := require.New(t)

	name := filepath.Join(fst.testRoot, "TestMkdirOnExistingDir")

	require.NoError(fst.fs.Mkdir(name, 0755))

	err := fst.fs.Mkdir(name, 0755)
	require.NotNil(err)
	assert.Condition(func() bool { return strings.HasSuffix(err.Error(), "file exists") }, "error does not match: %s", err.Error())
}

func (fst *fsTester) TestMkdirOnExistingFile(t *testing.T) {
	assert := assert.New(t)
	require := require.New(t)

	name := filepath.Join(fst.testRoot, "TestMkdirOnExistingFile")
	require.NoError(fst.fs.Mkdir(name, 0755))

	filename := filepath.Join(name, "testfile")
	f, err := fst.fs.CreateWriteCloser(filename)
	require.NoError(err)
	require.NoError(writeAll(f, []byte{0, 1, 2, 3, 4}))
	require.NoError(f.Close())

	err = fst.fs.Mkdir(filename, 0755)
	assert.Condition(func() bool { return strings.HasSuffix(err.Error(), "file exists") })
}

func (fst *fsTester) TestMkdirInNonExistingDir(t *testing.T) {
	assert := assert.New(t)

	name := filepath.Join(fst.testRoot, "TestMkdirInNonExistingDir")
	name = filepath.Join(name, "innerdir")
	err := fst.fs.Mkdir(name, 0755)

	assert.True(os.IsNotExist(err))
}

func (fst *fsTester) TestRemoveNonExistingAtRoot(t *testing.T) {
	assert := assert.New(t)

	err := fst.fs.Remove("not_existing_file")
	assert.True(os.IsNotExist(err))
}

func (fst *fsTester) TestRemoveNonExistingInSubdir(t *testing.T) {
	assert := assert.New(t)
	require := require.New(t)

	top := filepath.Join(fst.testRoot, "TestRemoveNonExistingInSubdir")
	require.NoError(fst.fs.Mkdir(top, 0755))

	err := fst.fs.Remove(filepath.Join(top, "not_existing_file"))
	assert.True(os.IsNotExist(err))
}

func (fst *fsTester) TestRemoveParentDirDoesNotExist(t *testing.T) {
	assert := assert.New(t)

	parent := filepath.Join(fst.testRoot, "TestRemoveParentDirDoesNotExist")
	child := filepath.Join(parent, "some_filename")

	err := fst.fs.Remove(child)
	assert.True(os.IsNotExist(err))
}

func (fst *fsTester) TestRemoveEmptyDir(t *testing.T) {
	assert := assert.New(t)
	require := require.New(t)

	name := filepath.Join(fst.testRoot, "TestRemoveEmptyDir")

	err := fst.fs.Mkdir(name, 0755)
	require.NoError(err)

	fi, err := fst.fs.Stat(name)
	assert.NoError(err)
	assert.NotNil(fi)

	assert.NoError(fst.fs.Remove(name))

	fi, err = fst.fs.Stat(name)
	assert.Nil(fi)
	require.NotNil(err)
	assert.True(os.IsNotExist(err))
}

func (fst *fsTester) TestRemoveNonEmptyDir(t *testing.T) {
	assert := assert.New(t)
	require := require.New(t)

	name := filepath.Join(fst.testRoot, "TestRemoveNonEmptyDir")

	err := fst.fs.Mkdir(name, 0755)
	require.NoError(err)

	w1, err := fst.fs.CreateWriteCloser(filepath.Join(name, "a_file"))
	require.NoError(err)
	assert.NoError(writeAll(w1, []byte{0, 1, 2, 3, 4}))
	assert.NoError(w1.Close())

	err = fst.fs.Remove(name)
	require.NotNil(err)
	assert.Condition(func() bool { return strings.HasSuffix(err.Error(), "directory not empty") }, "error does not match : %s", err.Error())

	fi, err := fst.fs.Stat(name)
	require.NoError(err)
	assert.Equal(fi.Name(), "TestRemoveNonEmptyDir")
}

func (fst *fsTester) TestRemoveFileInDir(t *testing.T) {
	assert := assert.New(t)
	require := require.New(t)

	dirname := filepath.Join(fst.testRoot, "TestRemoveFileInDir")
	filename := filepath.Join(dirname, "a_file")

	require.NoError(fst.fs.Mkdir(dirname, 0755))
	require.NoError(fst.writeFile(fst.fs, filename, []byte{1}))

	fi, err := fst.fs.Stat(filename)
	assert.NoError(err)
	assert.NotNil(fi)

	assert.NoError(fst.fs.Remove(filename))

	fi, err = fst.fs.Stat(filename)
	assert.Nil(fi)
	require.NotNil(err)
	assert.True(os.IsNotExist(err))
}

func (fst *fsTester) TestOverwrite(t *testing.T) {
	assert := assert.New(t)
	require := require.New(t)

	name := filepath.Join(fst.testRoot, "TestOverwrite")

	w1, err := fst.fs.CreateWriteCloser(name)
	require.NoError(err)
	assert.NotNil(w1)
	assert.NoError(writeAll(w1, []byte{0, 1, 2, 3, 4}))

	assert.NoError(w1.Close())

	r1, err := fst.fs.OpenReadCloser(name)
	require.NoError(err)
	assert.NotNil(r1)
	all, err := ioutil.ReadAll(r1)
	assert.NoError(err)
	assert.Equal([]byte{0, 1, 2, 3, 4}, all)

	w2, err := fst.fs.CreateWriteCloser(name)
	assert.NoError(err)
	assert.NotNil(w2)
	assert.NoError(writeAll(w2, []byte{5, 6, 7}))
	assert.NoError(w2.Close())

	r2, err := fst.fs.OpenReadCloser(name)
	assert.NoError(err)
	assert.NotNil(r2)
	all, err = ioutil.ReadAll(r2)
	assert.NoError(err)
	assert.Equal([]byte{5, 6, 7}, all)
}

func (fst *fsTester) TestReaddir(t *testing.T) {
	assert := assert.New(t)
	require := require.New(t)

	dir := filepath.Join(fst.testRoot, "TestReaddir")
	dir1 := filepath.Join(dir, "dir1")
	file1 := filepath.Join(dir, "file1")
	file2 := filepath.Join(dir1, "file2")

	require.NoError(fst.fs.Mkdir(dir, 0755))
	require.NoError(fst.fs.Mkdir(dir1, 0755))
	require.NoError(fst.writeFile(fst.fs, file1, []byte{1}))
	require.NoError(fst.writeFile(fst.fs, file2, []byte{2}))

	rd1, err := fst.fs.Readdir(dir)
	assert.NoError(err)
	assert.Equal(2, len(rd1))

	assert.Equal("dir1", rd1[0].Name())
	assert.Equal("file1", rd1[1].Name())

	rd2, err := fst.fs.Readdir(dir1)
	assert.NoError(err)
	assert.Equal(1, len(rd2))

	assert.Equal("file2", rd2[0].Name())
}

func (fst *fsTester) TestReaddirMoreThanMaxKeysFiles(t *testing.T) {
	// max keys defaults to 1000
	assert := assert.New(t)
	require := require.New(t)

	dir := filepath.Join(fst.testRoot, "TestReaddirManyFiles")
	require.NoError(fst.fs.Mkdir(dir, 0755))
	for i := 0; i < 1010; i++ {
		if i%100 == 0 {
			log.Printf("created %d files", i)
		}
		file := filepath.Join(dir, fmt.Sprintf("file%d", i))
		require.NoError(fst.writeFile(fst.fs, file, []byte{1}))
	}
	rd1, err := fst.fs.Readdir(dir)
	assert.NoError(err)
	require.Equal(1010, len(rd1))
}

func (fst *fsTester) TestStat(t *testing.T) {
	assert := assert.New(t)
	require := require.New(t)

	dir := filepath.Join(fst.testRoot, "TestStat")
	dir1 := filepath.Join(dir, "dir")
	file := filepath.Join(dir1, "file")

	require.NoError(fst.fs.Mkdir(dir, 0755))
	require.NoError(fst.fs.Mkdir(dir1, 0755))
	require.NoError(fst.writeFile(fst.fs, file, []byte{2}))

	fi, err := fst.fs.Stat(dir1)
	assert.NoError(err)
	assert.Equal(true, fi.IsDir())
	assert.Equal("dir", fi.Name())
	assert.Equal(os.FileMode(0755)|os.ModeDir, fi.Mode())
	assert.Equal(int64(4096), fi.Size())

	fi, err = fst.fs.Stat(file)
	assert.NoError(err)
	assert.Equal(false, fi.IsDir())
	assert.Equal("file", fi.Name())
	assert.Equal(os.FileMode(0644), fi.Mode())
	assert.Equal(int64(1), fi.Size())

	root := "/"
	fi, err = fst.fs.Stat(root)
	assert.NoError(err)
	assert.Equal(true, fi.IsDir())
}

func (fst *fsTester) TestReadAtBasic(t *testing.T) {
	assert := assert.New(t)
	require := require.New(t)

	dir := filepath.Join(fst.testRoot, "TestReadAtBasic")
	file := filepath.Join(dir, "file")

	data := make([]byte, 64)
	for i := range data {
		data[i] = byte(i)
	}

	require.NoError(fst.fs.Mkdir(dir, 0755))
	require.NoError(fst.writeFile(fst.fs, file, data))

	r, err := fst.fs.OpenReadCloser(file)
	require.NoError(err)
	assert.NotNil(r)

	buf := make([]byte, 1024)

	// read first 4 bytes
	i, err := r.ReadAt(buf[0:4], 0)
	assert.NoError(err)
	assert.Equal(4, i)
	assert.Equal(data[0:i], buf[0:i])

	// read second 4 bytes
	i, err = r.ReadAt(buf[0:4], 4)
	assert.NoError(err)
	assert.Equal(4, i)
	assert.Equal(data[4:i+4], buf[0:i])

	// read all bytes, but not past file end
	i, err = r.ReadAt(buf[0:len(data)], 0)
	assert.NoError(err)
	assert.Equal(len(data), i)
	assert.Equal(data, buf[0:i])

	// read all bytes, and past file
	i, err = r.ReadAt(buf, 0)
	assert.Equal(io.EOF, err)
	assert.Equal(len(data), i)
	assert.Equal(data, buf[0:i])

	// read first 4 bytes again, now that we've seen EOF.
	i, err = r.ReadAt(buf[0:4], 0)
	assert.NoError(err)
	assert.Equal(4, i)
	assert.Equal(data[0:i], buf[0:i])
}

func (fst *fsTester) TestReadAtWithRead(t *testing.T) {
	// This test shows that ReadAt does not move the file position Read sees.

	assert := assert.New(t)
	require := require.New(t)

	dir := filepath.Join(fst.testRoot, "TestReadAtWithRead")
	file := filepath.Join(dir, "file")

	data := make([]byte, 64)
	for i := range data {
		data[i] = byte(i)
	}

	require.NoError(fst.fs.Mkdir(dir, 0755))
	require.NoError(fst.writeFile(fst.fs, file, data))

	r, err := fst.fs.OpenReadCloser(file)
	require.NoError(err)
	assert.NotNil(r)

	buf := make([]byte, 1024)

	// read 8 bytes, using Read
	i, err := r.Read(buf[0:8])
	assert.NoError(err)
	assert.Equal(8, i)
	assert.Equal(data[0:i], buf[0:i])

	// read 4 bytes from position 40 using ReadAt
	i, err = r.ReadAt(buf[0:4], 20)
	assert.NoError(err)
	assert.Equal(4, i)
	assert.Equal(data[20:i+20], buf[0:i])

	// read 8 bytes, using Read
	i, err = r.Read(buf[0:8])
	assert.NoError(err)
	assert.Equal(8, i)
	assert.Equal(data[8:i+8], buf[0:i])

	// read to EOF, using Read
	i, err = r.Read(buf)
	// might get EOF now, or might be on next call.
	if err == io.EOF {
		assert.Equal(io.EOF, err)
		assert.Equal(48, i)
		assert.Equal(data[16:], buf[0:i])
	} else {
		i, err = r.Read(buf)
		assert.Equal(io.EOF, err)
		assert.Equal(0, i)
	}

	// readat should still work now.
	i, err = r.ReadAt(buf[0:4], 20)
	assert.NoError(err)
	assert.Equal(4, i)
	assert.Equal(data[20:i+20], buf[0:i])

}

func (fst *fsTester) TestSeek(t *testing.T) {
	assert := assert.New(t)
	require := require.New(t)

	dir := filepath.Join(fst.testRoot, "TestSeek")
	file := filepath.Join(dir, "file")

	data := make([]byte, 64)
	for i := range data {
		data[i] = byte(i)
	}

	require.NoError(fst.fs.Mkdir(dir, 0755))
	require.NoError(fst.writeFile(fst.fs, file, data))

	r, err := fst.fs.OpenReadCloser(file)
	require.NoError(err)
	assert.NotNil(r)

	buf := make([]byte, 1024)

	// read first 4 bytes
	i, err := r.ReadAt(buf[0:4], 0)
	assert.NoError(err)
	assert.Equal(4, i)
	assert.Equal(data[0:i], buf[0:i])

	// seek to 48, read 4 bytes
	pos, err := r.Seek(48, io.SeekStart)
	assert.NoError(err)
	assert.Equal(int64(48), pos)
	i, err = r.Read(buf[0:4])
	assert.NoError(err)
	assert.Equal(4, i)
	assert.Equal(data[48:i+48], buf[0:i])

	// seek to almost end, try to read past end
	pos, err = r.Seek(60, io.SeekStart)
	assert.NoError(err)
	assert.Equal(int64(60), pos)
	i, err = r.Read(buf[0:8])
	if err != nil && err != io.EOF {
		assert.NoError(err)
	}
	assert.Equal(4, i)
	assert.Equal(data[60:i+60], buf[0:i])

	// next read should be EOF
	i, err = r.Read(buf[0:8])
	assert.Equal(io.EOF, err)
	assert.Equal(0, i)

	// read first 4 bytes again, now that we've seen EOF.
	pos, err = r.Seek(0, io.SeekStart)
	assert.NoError(err)
	assert.Equal(int64(0), pos)
	i, err = r.Read(buf[0:4])
	assert.NoError(err)
	assert.Equal(4, i)
	assert.Equal(data[0:i], buf[0:i])

	// seek past end
	pos, err = r.Seek(128, io.SeekStart)
	assert.NoError(err)
	assert.Equal(int64(128), pos)
	i, err = r.Read(buf[0:4])
	assert.Equal(io.EOF, err)
	assert.Equal(0, i)
}

func (fst *fsTester) writeFile(fs straw.StreamStore, name string, data []byte) error {
	w, err := fs.CreateWriteCloser(name)
	if err != nil {
		return err
	}
	if err := writeAll(w, data); err != nil {
		w.Close()
		return err
	}
	return w.Close()
}

/*

func (fst *fsTester) TestAppend(t *testing.T) {
	assert := assert.New(t)

	name := filepath.Join(tempdir, "testAppend")

	f, err := fst.fs.OpenFile(name, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0666)
	assert.NoError(err)
	assert.NotNil(f)
	assert.NoError(writeAll(f, []byte{0, 1, 2, 3, 4}))
	assert.NoError(f.Close())

	f, err = fst.fs.OpenFile(name, os.O_RDONLY, 0)
	assert.NoError(err)
	assert.NotNil(f)
	all, err := ioutil.ReadAll(f)
	assert.NoError(err)
	assert.Equal([]byte{0, 1, 2, 3, 4}, all)

	f, err = fst.fs.OpenFile(name, os.O_RDWR|os.O_APPEND, 0666)
	assert.NoError(err)
	assert.NotNil(f)
	assert.NoError(writeAll(f, []byte{5, 6, 7}))
	assert.NoError(f.Close())

	f, err = fst.fs.OpenFile(name, os.O_RDONLY, 0)
	assert.NoError(err)
	assert.NotNil(f)
	all, err = ioutil.ReadAll(f)
	assert.NoError(err)
	assert.Equal([]byte{0, 1, 2, 3, 4, 5, 6, 7}, all)

}

func (fst *fsTester) TestWriteAtCreate(t *testing.T) {
	assert := assert.New(t)

	name := filepath.Join(tempdir, "testWriteAtCreate")
	f, err := fst.fs.OpenFile(name, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0666)
	assert.NoError(err)
	assert.NotNil(f)

	i, err := f.WriteAt([]byte{1, 2}, 14)
	assert.NoError(err)
	assert.Equal(2, i)
	assert.NoError(f.Close())

	fi, err := fst.fs.Stat(name)
	assert.NoError(err)
	assert.Equal(fi.Size(), int64(16))

	f, err = fst.fs.OpenFile(name, os.O_RDONLY, 0)
	assert.NoError(err)
	assert.NotNil(f)
	all, err := ioutil.ReadAll(f)
	assert.NoError(err)
	assert.Equal([]byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1, 2}, all)
}
*/

func writeAll(w io.Writer, data []byte) error {
	i, err := w.Write(data)
	if err != nil {
		return err
	}
	if i != len(data) {
		return io.ErrShortWrite
	}
	return nil
}

func tempDir() string {
	tempdir, err := ioutil.TempDir("", "straw_test_")
	if err != nil {
		panic(err)
	}
	return tempdir
}

func TestOSFS(t *testing.T) {
	osfs, err := straw.Open("file:///")
	if err != nil {
		t.Fatal(err)
	}
	testFS(t, "osfs", func() straw.StreamStore { return &TestLogStreamStore{t, osfs} }, tempDir())
}

func TestMemFS(t *testing.T) {
	ss, _ := straw.Open("mem://")
	testFS(t, "memfs", func() straw.StreamStore { return &TestLogStreamStore{t, ss} }, "/")
}

func TestS3FS(t *testing.T) {
	testBucket := os.Getenv("S3_TEST_BUCKET")
	if testBucket == "" {
		t.Skip("S3_TEST_BUCKET not set, skipping tests for s3 backend")
	}

	s3fs, err := straw.Open(fmt.Sprintf("s3://%s/", testBucket))
	if err != nil {
		t.Fatal(err)
	}
	testFS(t, "s3fs", func() straw.StreamStore { return &TestLogStreamStore{t, s3fs} }, "/")
}

func TestGCSFS(t *testing.T) {
	testBucket := os.Getenv("GCS_TEST_BUCKET")
	if testBucket == "" {
		t.Skip("GCS_TEST_BUCKET not set, skipping tests for gcs backend")
	}
	testGCSCredentials := os.Getenv("GCS_TEST_CREDENTIALS_FILE")
	if testGCSCredentials == "" {
		t.Skip("GCS_TEST_CREDENTIALS_FILE not set, skipping tests for gcs backend")
	}

	gcsFs, err := straw.Open(fmt.Sprintf("gs://%s/?credentialsfile=%s", testBucket, testGCSCredentials))
	if err != nil {
		t.Fatal(err)
	}
	testFS(t, "gcsfs", func() straw.StreamStore { return &TestLogStreamStore{t, gcsFs} }, "/")
}

func TestSFTPFS(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}

	go startSFTPServer(priv)

	sshKey, err := ssh.NewPublicKey(pub)
	if err != nil {
		t.Fatal(err)
	}

	encodedKey := base64.URLEncoding.EncodeToString(sshKey.Marshal())

	sftpfs, err := straw.Open("sftp://test:tiger@localhost:9922/?host_key=" + encodedKey)
	if err != nil {
		t.Fatal(err)
	}
	dir, err := ioutil.TempDir("", "straw_sftp_test")
	if err != nil {
		t.Fatal(err)
	}
	testFS(t, "sftpfs", func() straw.StreamStore { return &TestLogStreamStore{t, sftpfs} }, dir)
}

func startSFTPServer(priv ed25519.PrivateKey) {
	config := &ssh.ServerConfig{
		PasswordCallback: func(c ssh.ConnMetadata, pass []byte) (*ssh.Permissions, error) {
			log.Printf("Login: %s\n", c.User())
			if c.User() == "test" && string(pass) == "tiger" {
				return nil, nil
			}
			return nil, fmt.Errorf("password rejected for %q", c.User())
		},
	}

	private, err := ssh.NewSignerFromKey(priv)
	if err != nil {
		log.Fatal(err)
	}

	config.AddHostKey(private)

	listener, err := net.Listen("tcp", "0.0.0.0:9922")
	if err != nil {
		log.Fatal("failed to listen for connection", err)
	}
	log.Printf("Listening on %v\n", listener.Addr())

	nConn, err := listener.Accept()
	if err != nil {
		log.Fatal("failed to accept incoming connection", err)
	}

	_, chans, reqs, err := ssh.NewServerConn(nConn, config)
	if err != nil {
		log.Fatal("failed to handshake", err)
	}
	log.Printf("SSH server established\n")

	go ssh.DiscardRequests(reqs)

	for newChannel := range chans {
		log.Printf("Incoming channel: %s\n", newChannel.ChannelType())
		if newChannel.ChannelType() != "session" {
			newChannel.Reject(ssh.UnknownChannelType, "unknown channel type")
			log.Printf("Unknown channel type: %s\n", newChannel.ChannelType())
			continue
		}
		channel, requests, err := newChannel.Accept()
		if err != nil {
			log.Fatal("could not accept channel.", err)
		}
		log.Printf("Channel accepted\n")

		go func(in <-chan *ssh.Request) {
			for req := range in {
				log.Printf("Request: %v\n", req.Type)
				ok := false
				switch req.Type {
				case "subsystem":
					log.Printf("Subsystem: %s\n", req.Payload[4:])
					if string(req.Payload[4:]) == "sftp" {
						ok = true
					}
				}
				log.Printf(" - accepted: %v\n", ok)
				req.Reply(ok, nil)
			}
		}(requests)

		serverOptions := []sftp.ServerOption{
			sftp.WithDebug(log.Writer()),
		}

		server, err := sftp.NewServer(
			channel,
			serverOptions...,
		)
		if err != nil {
			log.Fatal(err)
		}
		if err := server.Serve(); err == io.EOF {
			server.Close()
			log.Print("sftp client exited session.")
		} else if err != nil {
			log.Fatal("sftp server completed with error:", err)
		}
	}
}

func testFS(t *testing.T, name string, fsProvider func() straw.StreamStore, rootDir string) {
	tester := &fsTester{name, nil, fsProvider, rootDir}

	typ := reflect.TypeOf(tester)
	val := reflect.ValueOf(tester)
	nm := typ.NumMethod()
	for i := 0; i < nm; i++ {
		mName := typ.Method(i).Name
		if strings.HasPrefix(mName, "Test") {
			tester.fs = tester.ff()
			t.Run(tester.name+"_"+mName, val.Method(i).Interface().(func(*testing.T)))
		}
	}
}

func TestMkdirAll(t *testing.T) {
	assert := assert.New(t)

	ss, _ := straw.Open("mem://")

	assert.NoError(straw.MkdirAll(ss, "/foo/bar/baz/qux/quux/", 0644))

	fis, err := ss.Readdir("/foo/bar/baz/qux/")
	assert.NoError(err)

	assert.Equal(1, len(fis))
	assert.Equal("quux", fis[0].Name())
}

func TestMkdirAllExistingNoError(t *testing.T) {
	assert := assert.New(t)

	ss, _ := straw.Open("mem://")

	assert.NoError(straw.MkdirAll(ss, "/foo/bar/baz/qux/quux/", 0644))
	assert.NoError(straw.MkdirAll(ss, "/foo/bar/baz/qux/quux/", 0644))
}
