package imagetest

import (
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"testing"

	"github.com/anchore/go-testutils"
	"github.com/anchore/stereoscope"
	"github.com/anchore/stereoscope/pkg/image"
	"github.com/logrusorgru/aurora"
)

const (
	CacheDir    = testutils.TestFixturesDir + string(filepath.Separator) + "cache"
	ImagePrefix = "stereoscope-fixture"
)

func GetFixtureImage(t *testing.T, source, name string) (*image.Image, func()) {
	t.Helper()

	sourceObj := image.ParseSourceScheme(source)

	var location string
	switch sourceObj {
	case image.DockerTarballSource:
		location = GetFixtureImageTarPath(t, name)
	case image.DockerDaemonSource:
		location = LoadFixtureImageIntoDocker(t, name)
	case image.OciTarballSource:
		dockerArchivePath := GetFixtureImageTarPath(t, name)
		ociArchivePath := path.Join(path.Dir(dockerArchivePath), "oci-archive-"+path.Base(dockerArchivePath))
		if _, err := os.Stat(ociArchivePath); os.IsNotExist(err) {
			skopeoCopyDockerArchiveToPath(t, dockerArchivePath, fmt.Sprintf("oci-archive:%s", ociArchivePath))
		}
		location = ociArchivePath
	case image.OciDirectorySource:
		dockerArchivePath := GetFixtureImageTarPath(t, name)
		ociDirPath := path.Join(path.Dir(dockerArchivePath), "oci-dir-"+path.Base(dockerArchivePath))
		if _, err := os.Stat(ociDirPath); os.IsNotExist(err) {
			skopeoCopyDockerArchiveToPath(t, dockerArchivePath, fmt.Sprintf("oci:%s", ociDirPath))
		}
		location = ociDirPath
	default:
		t.Fatalf("could not determine source: %+v", source)
	}
	request := fmt.Sprintf("%s:%s", source, location)

	i, err := stereoscope.GetImage(request)
	if err != nil {
		t.Fatal("could not get tar image:", err)
	}

	return i, stereoscope.Cleanup
}

func GetGoldenFixtureImage(t *testing.T, name string) *image.Image {
	t.Helper()

	imageName, _ := getFixtureImageInfo(t, name)
	tarFileName := imageName + testutils.GoldenFileExt
	tarPath := getFixtureImageTarPath(t, name, testutils.GoldenFileDirPath, tarFileName)
	return getFixtureImageFromTar(t, tarPath)
}

func UpdateGoldenFixtureImage(t *testing.T, name string) {
	t.Helper()

	t.Log(aurora.Reverse(aurora.Red("!!! UPDATING GOLDEN FIXTURE IMAGE !!!")), name)

	imageName, _ := getFixtureImageInfo(t, name)
	goldenTarFilePath := path.Join(testutils.GoldenFileDirPath, imageName+testutils.GoldenFileExt)
	tarPath := GetFixtureImageTarPath(t, name)
	copyFile(t, tarPath, goldenTarFilePath)
}

func isSkopeoAvailable() bool {
	_, err := exec.LookPath("skopeo")
	return err == nil
}

func skopeoCopyDockerArchiveToPath(t *testing.T, dockerArchivePath, destination string) {
	if !isSkopeoAvailable() {
		t.Fatalf("cannot find skopeo executable")
	}

	archive := fmt.Sprintf("docker-archive:%s", dockerArchivePath)
	cmd := exec.Command("skopeo", "copy", "--insecure-policy", archive, destination)
	cmd.Env = os.Environ()

	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin

	if err := cmd.Run(); err != nil {
		t.Fatalf("skopeo failed: %+v", err)
	}
}

func getFixtureImageFromTar(t *testing.T, tarPath string) *image.Image {
	t.Helper()

	request := fmt.Sprintf("docker-archive:%s", tarPath)

	i, err := stereoscope.GetImage(request)
	if err != nil {
		t.Fatal("could not get tar image:", err)
	}

	return i
}

func getFixtureImageInfo(t *testing.T, name string) (string, string) {
	t.Helper()
	version := fixtureVersion(t, name)
	imageName := fmt.Sprintf("%s-%s", ImagePrefix, name)
	return imageName, version
}

func LoadFixtureImageIntoDocker(t *testing.T, name string) string {
	t.Helper()
	imageName, imageVersion := getFixtureImageInfo(t, name)
	fullImageName := fmt.Sprintf("%s:%s", imageName, imageVersion)

	if !hasImage(t, fullImageName) {
		contextPath := path.Join(testutils.TestFixturesDir, name)
		err := buildImage(t, contextPath, imageName, imageVersion)
		if err != nil {
			t.Fatal("could not build fixture image:", err)
		}
	}

	return fullImageName
}

func getFixtureImageTarPath(t *testing.T, fixtureName, tarStoreDir, tarFileName string) string {
	t.Helper()
	imageName, imageVersion := getFixtureImageInfo(t, fixtureName)
	fullImageName := fmt.Sprintf("%s:%s", imageName, imageVersion)
	tarPath := path.Join(tarStoreDir, tarFileName)

	// create the cache dir if it does not already exist...
	if !fileOrDirExists(t, CacheDir) {
		err := os.Mkdir(CacheDir, 0o755)
		if err != nil {
			t.Fatalf("could not create tar cache dir (%s): %+v", CacheDir, err)
		}
	}

	// if the image tar does not exist, make it
	if !fileOrDirExists(t, tarPath) {
		if !hasImage(t, fullImageName) {
			contextPath := path.Join(testutils.TestFixturesDir, fixtureName)
			err := buildImage(t, contextPath, imageName, imageVersion)
			if err != nil {
				t.Fatal("could not build fixture image:", err)
			}
		}

		err := saveImage(t, fullImageName, tarPath)
		if err != nil {
			t.Fatal("could not save fixture image:", err)
		}
	}

	return tarPath
}

func GetFixtureImageTarPath(t *testing.T, name string) string {
	t.Helper()
	imageName, imageVersion := getFixtureImageInfo(t, name)
	tarFileName := fmt.Sprintf("%s-%s.tar", imageName, imageVersion)
	return getFixtureImageTarPath(t, name, CacheDir, tarFileName)
}

func fixtureVersion(t *testing.T, name string) string {
	t.Helper()
	contextPath := path.Join(testutils.TestFixturesDir, name)
	dockerfileHash, err := dirHash(t, contextPath)
	if err != nil {
		panic(err)
	}
	return dockerfileHash
}

func hasImage(t *testing.T, imageName string) bool {
	t.Helper()
	cmd := exec.Command("docker", "image", "inspect", imageName)
	cmd.Env = os.Environ()
	err := cmd.Run()
	return err == nil
}

func buildImage(t *testing.T, contextDir, name, tag string) error {
	t.Helper()
	fullTag := fmt.Sprintf("%s:%s", name, tag)
	latestTag := fmt.Sprintf("%s:latest", name)
	cmd := exec.Command("docker", "build", "-t", fullTag, "-t", latestTag, ".")
	cmd.Env = os.Environ()
	cmd.Dir = contextDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	return cmd.Run()
}

func saveImage(t *testing.T, image, path string) error {
	t.Helper()

	outfile, err := os.Create(path)
	if err != nil {
		t.Fatal("unable to create file for docker image tar:", err)
	}
	defer func() {
		err := outfile.Close()
		if err != nil {
			panic(err)
		}
	}()

	// note: we are not using -o since some CI providers need root access for the docker client, however,
	// we don't want the resulting tar to be owned by root, thus we write the file piped from stdout.
	cmd := exec.Command("docker", "image", "save", image)
	cmd.Env = os.Environ()

	cmd.Stdout = outfile
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	return cmd.Run()
}
