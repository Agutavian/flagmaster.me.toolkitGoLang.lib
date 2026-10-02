package FlagmasterToolkitImageManipulatorLib

import (
	"log"
	"os"
	"path/filepath"
	"testing"
)

//
//import (
//	"fmt"
//	"log"
//	"testing"
//)
//
//func TestGetPercentageCompleted(t *testing.T) {
//	userAgent := "Mozilla/5.0 (X11; Linux x86_64; rv:139.0) Gecko/20100101 Firefox/139.0"
//	percentage, err := GetPercentageCompleted(userAgent, true)
//	if err != nil {
//		log.Fatal(err)
//	}
//	fmt.Println(percentage)
//}

func Test_jsonFinalCompiler(t *testing.T) {
	imagePath := "testImages"
	imageMap := make(map[string][]string)
	var testImages []string
	testImages = append(testImages, "DSCF2216.JPG")
	imageMap["testing_category"] = testImages
	compressedImagesBytes, finalJson, err := ImageManipulator(imagePath, imageMap)
	if err != nil {
		t.Errorf("Error Manipulating ImageManipulator: %v", err)
	}
	jsonFile := "testImages/output/TEST_output_exif_data.json"
	//exifJson, err := json.Marshal(results)

	_, _ = os.Create(jsonFile)
	err = os.WriteFile(jsonFile, finalJson, 0644)

	for fileName, data := range compressedImagesBytes {
		outPath := filepath.Join("testImages/output/", fileName+".webp")
		if err := os.WriteFile(outPath, data, 0o644); err != nil {
			log.Printf("writing %s: %v", outPath, err)
			continue
		}
	}

}
