package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"image/jpeg"
	"io"
	"io/fs"
	"log"
	"math"
	"os"
	"strings"
	"sync"

	gometadata "github.com/FlavioCFOliveira/GoMetadata"
	"github.com/FlavioCFOliveira/GoMetadata/xmp"
	"github.com/kolesa-team/go-webp/encoder"
	"github.com/kolesa-team/go-webp/webp"
	//"github.com/rwcarlsen/goexif/exif"
)

const nsMyApp = "https://flagmaster.me" // your own namespace URI

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

// path: the absolute path to the files
func imageManipulator(path string) {
	// I guess this is where it starts off from?
	myFs := os.DirFS(".")
	entries, err := fs.ReadDir(myFs, path)
	if err != nil {
		panic(err)
	}

	var exifDataEntries [][]byte

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
			result := processImage(entry.Name(), path)
			mutex.Lock()
			exifDataEntries = append(exifDataEntries, result)
			mutex.Unlock()
		})
	}
	waitGroup.Wait()
	fmt.Printf("Exif making sure: %s\n", exifDataEntries)

	finalJson := func() []byte {
		raw := make([]json.RawMessage, len(exifDataEntries))
		for i, b := range exifDataEntries {
			raw[i] = b
		}
		jsonMarshal, _ := json.MarshalIndent(raw, "", "\t")
		if err != nil {
			log.Fatal(err)
		}
		return jsonMarshal
	}()

	jsonFile := path + "/output/" + "output_exif_data" + ".json"

	//output, err := os.Create(jsonFile)
	if err != nil {
		log.Fatal(err)
	}

	err = os.WriteFile(jsonFile, finalJson, 0644)
	if err != nil {
		return
	}

}

func processImage(entryName string, path string) []byte {
	fmt.Println("Processing " + entryName)
	//open the file

	imagePath := path + "/" + entryName
	imageFile, err := os.Open(imagePath)
	if err != nil {
		log.Fatal(err)
	}
	exifData, err := getExifData(entryName, imagePath, imageFile)

	//rewinds the cursor so the image can decode proberly
	if _, err := imageFile.Seek(0, io.SeekStart); err != nil {
		log.Fatal(err)
	}

	imageToWEBP(path+"/output/", entryName, imageFile)

	//err = addImageCategory(path+"/output/"+entryName+".webp", "ineedtosleep")
	//if err != nil {
	//	log.Fatal(err)
	//}
	//outputCategory, err := getImageCategory(path + "/output/" + entryName + ".webp")
	//if err != nil {
	//	log.Fatal(err)
	//}
	//println("output category: " + outputCategory)
	//err = removeImageCategory(path + "/output/" + entryName + ".webp")
	//if err != nil {
	//	return nil
	//}
	//outputCategory, err = getImageCategory(path + "/output/" + entryName + ".webp")
	//println("output category after removal: " + outputCategory)

	err = imageFile.Close()
	if err != nil {
		return nil
	}

	exifJson, err := json.Marshal(exifData)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println("Done Processing " + entryName)

	return exifJson
}

// Path is where it should be outputted, filename is the name of the output file, image is the image to be converted
func imageToWEBP(path string, filename string, image *os.File) {
	decodedJpeg, err := jpeg.Decode(image)
	if err != nil {
		log.Fatal(err)
	}

	output, err := os.Create(path + filename + ".webp")
	if err != nil {
		log.Fatal(err)
	}
	defer func(output *os.File) {
		err := output.Close()
		if err != nil {

		}
	}(output)

	options, err := encoder.NewLossyEncoderOptions(encoder.PresetDefault, 10)
	if err != nil {
		log.Fatalln(err)
	}

	if err := webp.Encode(output, decodedJpeg, options); err != nil {
		log.Fatalln(err)
	}

}

// library info: https://github.com/FlavioCFOliveira/GoMetadata
func getExifData(imageName string, imagePath string, imageFile *os.File) (ExifDataReceived, error) {
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

		var ModelStr string
		ModelStr = exifMetadata.Make()
		if err != nil {
			ModelStr = "Unknown"
		}

		if MakeStr == "Unknown" || ModelStr == "Unknown" {
			return "Unknown"
		}

		return MakeStr + " " + ModelStr

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
			return math.NaN()
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

// ImagePath is the absolute path to the image. Category Name is the category you want to name
func addImageCategory(imagePath string, categoryName string) error {
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

// Removes the image category from an image
func removeImageCategory(imagePath string) error {
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

func getImageCategory(imagePath string) (string, error) {
	metadata, err := gometadata.ReadFile(imagePath)
	if err != nil {
		log.Fatal(err)
	}
	if metadata.XMP == nil {
		return "", errors.New("no XMP metadata")
	}

	categoryName := metadata.XMP.Get(nsMyApp, "Category")
	println()

	return categoryName, nil
}
