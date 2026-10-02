package FlagmasterToolkitImageManipulatorLib

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"image/jpeg"
	"io"
	"io/fs"
	"log"
	"os"
	"strings"
	"sync"

	gometadata "github.com/FlavioCFOliveira/GoMetadata"
	"github.com/FlavioCFOliveira/GoMetadata/xmp"
	"github.com/kolesa-team/go-webp/encoder"
	"github.com/kolesa-team/go-webp/webp"
	//"github.com/rwcarlsen/goexif/exif"
)

// The app's namespace for XMP data
const nsMyApp = "https://flagmaster.me"

// ExifDataReceived Exif struct
type ExifDataReceived struct {
	//take a wild guess what this means
	FileName string `json:"FileName"`

	//make and model of camera
	MakeModel string `json:"MakeModel"`
	//Model string `json:"model"`

	//Lens
	Lens string `json:"Lens"`
	//LensModel       string `json:"lensModel"`
	ImageDimensions ImageDimensionsStruct `json:"ImageDimensions"`
	//PixelYDimension int64  `json:"PixelYDimension"`

	//Picture stuff
	ExposureTime    string  `json:"ExposureTime"`    // [string]
	FStopValue      float64 `json:"ApertureValue"`   // [string]
	ISOSpeedRatings uint    `json:"ISOSpeedRatings"` // [int]
}

type ImageDimensionsStruct struct {
	X uint32
	Y uint32
}

// ImageManipulator path: the absolute path to the files
//
// Params:
//
// imagesPath: where the images to be processed should be
//
// jsonOutputPath: where the JSON should be outputted to.
//
// jsonFileName: JSON File name. Recommended to include ".json" at the end
//
// Return:
// map[string][]byte: Compressed Images map with key = filename, byte = compressedImage bytes
// []byte: Final JSON as bytes (already marshalled). Nil if error
// error: Error if applicable, nil otherwise
//
// INFO:
//
// Function implements GoRoutines to loop through images and process them.

func ImageManipulator(imagesPath string, imageMap map[string][]string) (compressedImagesBytes map[string][]byte, FinalJson []byte, error error) {
	myFs := os.DirFS(imagesPath)          // root the FS at the directory
	entries, err := fs.ReadDir(myFs, ".") // "." = the root of that FS
	if err != nil {
		return nil, nil, fmt.Errorf("reading images dir %q: %w", imagesPath, err)
	}

	var exifDataEntries []ExifDataReceived
	compressedImages := make(map[string][]byte)
	var waitGroup sync.WaitGroup
	var mutex sync.Mutex

	for index, entry := range entries {
		if entry.IsDir() {
			fmt.Println(entry.Name(), index, "DIRECTORY")
			continue
		}
		waitGroup.Add(1)
		//adds to async group
		waitGroup.Go(func() {
			defer waitGroup.Done()
			resultBytes, resultExif, err := ProcessImage(entry.Name(), imagesPath)
			if err != nil {
				fmt.Printf("Error processing image: %v\n", err)
			}
			mutex.Lock()
			exifDataEntries = append(exifDataEntries, resultExif)
			compressedImages[entry.Name()] = resultBytes
			mutex.Unlock()
		})
		//	test
	}
	waitGroup.Wait()
	fmt.Printf("Exif making sure: %#v\n", exifDataEntries)

	finalJson, err := _jsonFinalCompiler(imageMap, exifDataEntries)

	//jsonFile := jsonOutputPath + jsonFileName

	//output, err := os.Create(jsonFile)
	if err != nil {
		return nil, nil, err
	}

	return compressedImages, finalJson, nil
}

// imageMap is a list of the categories with the image names associated with them
// exifDataEntries is the list of already-compiled ExifDataReceived structs
func _jsonFinalCompiler(imageMap map[string][]string, exifDataEntries []ExifDataReceived) ([]byte, error) {

	finalJson, err := func() ([]byte, error) {
		exifMap := make(map[string][]ExifDataReceived)

		//raw := make([]json.RawMessage, len(exifDataEntries))
		//Loop through all categories
		for category, listOfImagesInCategory := range imageMap {

			// Loop through all the images in the category
			for _, imageName := range listOfImagesInCategory {
				//Get the exif data for this image specifically
				imageEXIF := func(imageName string) ExifDataReceived {
					// loop through all exifDataEntries
					for _, imageExifData := range exifDataEntries {
						//Check if the current image index is the same as the one in the current ExifDataReceived instance
						if imageExifData.FileName == imageName {
							return imageExifData
						}
					}
					fmt.Println("No EXIF data for " + imageName + " . Function _jsonFinalCompiler()")
					return ExifDataReceived{}
				}(imageName)
				fmt.Printf("Exif Map: %#v\n", exifMap)

				// Append exif data to the category
				exifMap[category] = append(exifMap[category], imageEXIF)
			}
		}
		jsonMarshal, _ := json.MarshalIndent(exifMap, "", "\t")

		return jsonMarshal, nil
	}()
	if err != nil {
		return nil, err
	}

	return finalJson, nil
}

// ProcessImageToWebP

// Path is where the image should be outputted
// Compresses image and gets EXIF data. Returns compressed file bytes and outputs EXIF data as ExifDataRecived
func ProcessImage(entryName string, path string) ([]byte, ExifDataReceived, error) {
	fmt.Println("Processing " + entryName)
	//open the file

	imagePath := path + "/" + entryName
	imageFile, err := os.Open(imagePath)
	if err != nil {
		return nil, ExifDataReceived{}, err
	}
	exifData, err := GetExifData(entryName, imagePath, imageFile)
	if err != nil {
		return nil, ExifDataReceived{}, err
	}

	//rewinds the cursor so the image can decode properly
	if _, err := imageFile.Seek(0, io.SeekStart); err != nil {
		return nil, ExifDataReceived{}, err
	}

	bytes, err := imageToWEBP(path, entryName, imageFile)
	if err != nil {
		return nil, ExifDataReceived{}, err
	}
	err = imageFile.Close()
	//if err != nil {
	//	return nil
	//}

	//exifJson, err := json.Marshal(exifData)
	if err != nil {
		return nil, ExifDataReceived{}, err
	}

	fmt.Println("Done Processing " + entryName + " to WEBP ")

	return bytes, exifData, nil
}

// Path is where it should be outputted, filename is the name of the output file, image is the image to be converted
// Returns compressed File bytes
func imageToWEBP(path string, filename string, image *os.File) ([]byte, error) {
	decodedJpeg, err := jpeg.Decode(image)
	if err != nil {
		return nil, err
	}

	//output, err := os.Create(path + filename + ".webp")
	//if err != nil {
	//	log.Fatal(err)
	//}
	//defer func(output *os.File) {
	//	err := output.Close()
	//	if err != nil {
	//
	//	}
	//}(output)

	options, err := encoder.NewLossyEncoderOptions(encoder.PresetDefault, 10)
	if err != nil {
		return nil, err
	}

	var buf bytes.Buffer
	if err := webp.Encode(&buf, decodedJpeg, options); err != nil {
		return nil, err
	}

	fmt.Println("Saved image to: " + path + filename + ".webp")

	return buf.Bytes(), nil
}

// library info: https://github.com/FlavioCFOliveira/GoMetadata

// GetExifData Returns ExifData for the image in the path. FIle is opened file already
func GetExifData(imageName string, imagePath string, imageFile *os.File) (ExifDataReceived, error) {
	//initialise empty struct
	var exifData ExifDataReceived

	exifMetadata, err := gometadata.ReadFile(imagePath,
		gometadata.WithoutMakerNote(),
		gometadata.WithoutIPTC(),
		gometadata.WithoutXMP(),
	)

	//get the exif data class from image file
	//exifMetadata, err := exif.Decode(imageFile)
	if exifMetadata == nil {
		return ExifDataReceived{}, errors.New("no EXIF Metadata @" + imageFile.Name())
	}
	if err != nil {
		log.Fatal(err)
	}

	//convert the exif data class to JSON. Using the json.marshal (convert data -> JSON)
	//exifJson, err := exifMetadata.MarshalJSON()

	// Uniquly: don't return empty string if unknown, as camera model is important to know

	exifData.FileName = imageName

	exifData.MakeModel = func() string {
		var MakeStr string
		MakeStr = exifMetadata.CameraModel()
		if err != nil {
			MakeStr = "Unknown"
		}
		//
		//var ModelStr string
		//ModelStr = exifMetadata.Make()
		//if err != nil {
		//	ModelStr = "Unknown"
		//}

		//if MakeStr == "Unknown" {
		//	return "Unknown"
		//}

		return MakeStr

	}()

	exifData.Lens = func() string {
		//var LensMake string
		//LensMake = exifMetadata.LensModel()
		//if err != nil {
		//	LensMake = ""
		//}

		var LensModel string
		LensModel = exifMetadata.LensModel()
		if err != nil {
			LensModel = ""
		}

		//if LensMake == "" || LensModel == "" {
		//	return ""
		//}

		//LensMake = strings.TrimSpace(LensMake)
		LensModel = strings.TrimSpace(LensModel)

		//Check if the lens make contains the model name already (i.e, Sigma 400 doesn't need to be called Sigma Sigma 400)
		//if strings.Contains(strings.ToLower(LensModel), strings.ToLower(LensMake)) {
		//	return LensModel
		//}
		return LensModel

	}()

	exifData.ImageDimensions = func() ImageDimensionsStruct {
		var ImagesDimension ImageDimensionsStruct

		PixelXDimension, PixelYDimension, ok := exifMetadata.ImageSize()
		if !ok {
			fmt.Println("no image size")
			return ImagesDimension
		}
		//, err2 := exifMetadata.Get(exif.PixelYDimension)

		//if err1 != nil || err2 != nil {
		//	return ImageDimensionsStruct{X: int64(math.NaN()), Y: int64(math.NaN())}
		//}

		//PixelXDimension, err1 := PixelXDimensionTiff.Int64(0)
		//PixelYDimension, err2 := PixelYDimensionTiff.Int64(0)

		//if err1 != nil || err2 != nil {
		//	return ImageDimensionsStruct{X: int64(math.NaN()), Y: int64(math.NaN())}
		//}

		ImagesDimension.X = PixelXDimension
		ImagesDimension.Y = PixelYDimension

		return ImagesDimension
	}()

	exifData.ExposureTime = func() string {
		ExposureTimeNumerator, ExposureTimeDenominator, ok := exifMetadata.ExposureTime()
		if ExposureTimeNumerator == 0 || ExposureTimeDenominator == 0 || !ok {
			return ""
		}
		top := float64(ExposureTimeNumerator) / float64(ExposureTimeNumerator)
		bottom := float64(ExposureTimeDenominator) / float64(ExposureTimeNumerator)
		return fmt.Sprint(top) + "/" + fmt.Sprint(bottom)
	}()

	exifData.FStopValue = func() float64 {
		FStop, ok := exifMetadata.FNumber()
		if !ok {
			return 0
		}
		//FStopNumerator, FStopDenominator, err := FStopTiff.Rat2(0) // retrieve first (hopefully only...) value from list
		//if FStopNumerator == 0 || FStopDenominator == 0 || err != nil {
		//	return math.NaN()
		//}
		//return float64(FStopNumerator) / float64(FStopDenominator)
		return FStop
	}()

	exifData.ISOSpeedRatings = func() uint {
		ISOSpeed, _ := exifMetadata.ISO()
		//if err != nil {
		//	return int64(math.NaN())
		//}
		//ISOValue, _ := ISOSpeedTiff.Int64(0)
		//if err != nil {
		//	return int64(math.NaN())
		//}
		return ISOSpeed
	}()

	return exifData, nil
}

// AddImageCategory : Add a category to the XMP data of image
// ImagePath is the absolute path to the image. Category Name is the category you want to name
func AddImageCategory(imagePath string, categoryName string) error {
	metadata, err := gometadata.ReadFile(imagePath)

	if err != nil {
		log.Fatal(err)
	}

	if metadata.XMP == nil {
		metadata.XMP = &xmp.XMP{}
	}

	metadata.XMP.Set(nsMyApp, "Category", categoryName)
	if err := gometadata.WriteFile(imagePath, metadata); err != nil {
		log.Fatal(err)
	}

	return nil
}

// RemoveImageCategory : Remove a category to the XMP data of image
// Removes the image category from an image
func RemoveImageCategory(imagePath string) error {
	metadata, err := gometadata.ReadFile(imagePath)

	if err != nil {
		log.Fatal(err)
	}

	if metadata.XMP == nil {
		return errors.New("no XMP metadata")
	}

	metadata.XMP.Set(nsMyApp, "Category", "")
	if err := gometadata.WriteFile(imagePath, metadata); err != nil {
		log.Fatal(err)
	}
	return nil
}

// GetImageCategory returns the image category string and a possible error. Blank/no category will return empty string ""
func GetImageCategory(imagePath string) (string, error) {
	metadata, err := gometadata.ReadFile(imagePath)
	if err != nil {
		return "", err
	}
	if metadata.XMP == nil {
		return "", nil
	}

	categoryName := metadata.XMP.Get(nsMyApp, "Category")
	println()

	return categoryName, nil
}
