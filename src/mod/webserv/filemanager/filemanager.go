package filemanager

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"imuslab.com/zoraxy/mod/utils"
)

/*
	File Manager

	This is a simple package that handles file management
	under the web server directory
*/

type FileManager struct {
	Directory string
}

// Create a new file manager with directory as root
func NewFileManager(directory string) *FileManager {
	return &FileManager{
		Directory: directory,
	}
}

// Handle listing of a given directory
func (fm *FileManager) HandleList(w http.ResponseWriter, r *http.Request) {
	directory, err := utils.GetPara(r, "dir")
	if err != nil {
		utils.SendErrorResponse(w, "invalid directory given")
		return
	}

	// Construct the absolute path to the target directory
	targetDir := filepath.Join(fm.Directory, directory)

	// Clean path to prevent path escape #274
	targetDir = filepath.ToSlash(filepath.Clean(targetDir))
	targetDir = strings.ReplaceAll(targetDir, "../", "")

	// Open the target directory
	dirEntries, err := os.ReadDir(targetDir)
	if err != nil {
		utils.SendErrorResponse(w, "unable to open directory")
		return
	}

	// Create a slice to hold the file information
	var files []map[string]interface{} = []map[string]interface{}{}

	// Iterate through the directory entries
	for _, dirEntry := range dirEntries {
		fileInfo := make(map[string]interface{})
		fileInfo["filename"] = dirEntry.Name()
		fileInfo["filepath"] = filepath.Join(directory, dirEntry.Name())
		fileInfo["isDir"] = dirEntry.IsDir()

		// Get file size and last modified time
		finfo, err := dirEntry.Info()
		if err != nil {
			//unable to load its info. Skip this file
			continue
		}
		fileInfo["lastModified"] = finfo.ModTime().Unix()
		if !dirEntry.IsDir() {
			// If it's a file, get its size
			fileInfo["size"] = finfo.Size()
		} else {
			// If it's a directory, set size to 0
			fileInfo["size"] = 0
		}

		// Append file info to the list
		files = append(files, fileInfo)
	}

	// Serialize the file info slice to JSON
	jsonData, err := json.Marshal(files)
	if err != nil {
		utils.SendErrorResponse(w, "unable to marshal JSON")
		return
	}

	// Set response headers and send the JSON response
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write(jsonData)
}

// Handle upload of a file (multi-part), 25MB max
func (fm *FileManager) HandleUpload(w http.ResponseWriter, r *http.Request) {
	dir, err := utils.PostPara(r, "dir")
	if err != nil {
		log.Println("no dir given")
		utils.SendErrorResponse(w, "invalid dir given")
		return
	}

	// Parse the multi-part form data
	err = r.ParseMultipartForm(25 << 20)
	if err != nil {
		utils.SendErrorResponse(w, "unable to parse form data")
		return
	}

	// Get the uploaded file
	file, fheader, err := r.FormFile("file")
	if err != nil {
		log.Println(err.Error())
		utils.SendErrorResponse(w, "unable to get uploaded file")
		return
	}
	defer file.Close()

	// Specify the directory where you want to save the uploaded file
	uploadDir := filepath.Join(fm.Directory, dir)
	if !utils.FileExists(uploadDir) {
		utils.SendErrorResponse(w, "upload target directory not exists")
		return
	}

	filename := sanitizeFilename(fheader.Filename)
	if !isValidFilename(filename) {
		utils.SendErrorResponse(w, "filename contain invalid or reserved characters")
		return
	}

	// Create the file on the server
	filePath := filepath.Join(uploadDir, filepath.Base(filename))
	out, err := os.Create(filePath)
	if err != nil {
		utils.SendErrorResponse(w, "unable to create file on the server")
		return
	}
	defer out.Close()

	// Copy the uploaded file to the server
	_, err = io.Copy(out, file)
	if err != nil {
		utils.SendErrorResponse(w, "unable to copy file to server")
		return
	}

	// Respond with a success message or appropriate response
	utils.SendOK(w)
}

// Handle download of a selected file, serve with content dispose header
func (fm *FileManager) HandleDownload(w http.ResponseWriter, r *http.Request) {
	filename, err := utils.GetPara(r, "file")
	if err != nil {
		utils.SendErrorResponse(w, "invalid filepath given")
		return
	}

	previewMode, _ := utils.GetPara(r, "preview")
	if previewMode == "true" {
		// Serve the file using http.ServeFile
		filePath := filepath.Join(fm.Directory, filename)
		http.ServeFile(w, r, filePath)
	} else {
		// Trigger a download with content disposition headers
		filePath := filepath.Join(fm.Directory, filename)
		w.Header().Set("Content-Disposition", "attachment; filename="+filepath.Base(filename))
		http.ServeFile(w, r, filePath)
	}
}

// HandleNewFolder creates a new folder in the specified directory
func (fm *FileManager) HandleNewFolder(w http.ResponseWriter, r *http.Request) {
	// Parse the directory name from the request
	dirName, err := utils.PostPara(r, "path")
	if err != nil {
		utils.SendErrorResponse(w, "invalid directory name")
		return
	}

	//Prevent path escape
	dirName = strings.ReplaceAll(dirName, "\\", "/")
	dirName = strings.ReplaceAll(dirName, "../", "")

	// Specify the directory where you want to create the new folder
	newFolderPath := filepath.Join(fm.Directory, dirName)

	// Check if the folder already exists
	if _, err := os.Stat(newFolderPath); os.IsNotExist(err) {
		// Create the new folder
		err := os.Mkdir(newFolderPath, os.ModePerm)
		if err != nil {
			utils.SendErrorResponse(w, "unable to create the new folder")
			return
		}

		// Respond with a success message or appropriate response
		utils.SendOK(w)
	} else {
		// If the folder already exists, respond with an error
		utils.SendErrorResponse(w, "folder already exists")
	}
}

// HandleFileCopy copies a file or directory from the source path to the destination path
func (fm *FileManager) HandleFileCopy(w http.ResponseWriter, r *http.Request) {
	// Parse the source and destination paths from the request
	srcPath, err := utils.PostPara(r, "srcpath")
	if err != nil {
		utils.SendErrorResponse(w, "invalid source path")
		return
	}

	destPath, err := utils.PostPara(r, "destpath")
	if err != nil {
		utils.SendErrorResponse(w, "invalid destination path")
		return
	}

	// Validate and sanitize the source and destination paths
	srcPath = filepath.Clean(srcPath)
	destPath = filepath.Clean(destPath)

	// Construct the absolute paths
	absSrcPath := filepath.Join(fm.Directory, srcPath)
	absDestPath := filepath.Join(fm.Directory, destPath)

	// Check if the source path exists
	if _, err := os.Stat(absSrcPath); os.IsNotExist(err) {
		utils.SendErrorResponse(w, "source path does not exist")
		return
	}

	// Check if the destination path exists
	if _, err := os.Stat(absDestPath); os.IsNotExist(err) {
		utils.SendErrorResponse(w, "destination path does not exist")
		return
	}

	//Join the name to create final paste filename
	absDestPath = filepath.Join(absDestPath, filepath.Base(absSrcPath))
	//Reject opr if already exists
	if utils.FileExists(absDestPath) {
		utils.SendErrorResponse(w, "target already exists")
		return
	}

	// Perform the copy operation based on whether the source is a file or directory
	if isDir(absSrcPath) {
		// Recursive copy for directories
		err := copyDirectory(absSrcPath, absDestPath)
		if err != nil {
			utils.SendErrorResponse(w, fmt.Sprintf("error copying directory: %v", err))
			return
		}
	} else {
		// Copy a single file
		err := copyFile(absSrcPath, absDestPath)
		if err != nil {
			utils.SendErrorResponse(w, fmt.Sprintf("error copying file: %v", err))
			return
		}
	}

	utils.SendOK(w)
}

func (fm *FileManager) HandleFileMove(w http.ResponseWriter, r *http.Request) {
	// Parse the source and destination paths from the request
	srcPath, err := utils.PostPara(r, "srcpath")
	if err != nil {
		utils.SendErrorResponse(w, "invalid source path")
		return
	}

	destPath, err := utils.PostPara(r, "destpath")
	if err != nil {
		utils.SendErrorResponse(w, "invalid destination path")
		return
	}

	// Validate and sanitize the source and destination paths
	srcPath = filepath.Clean(srcPath)
	destPath = filepath.Clean(destPath)

	// Construct the absolute paths
	absSrcPath := filepath.Join(fm.Directory, srcPath)
	absDestPath := filepath.Join(fm.Directory, destPath)

	// Check if the source path exists
	if _, err := os.Stat(absSrcPath); os.IsNotExist(err) {
		utils.SendErrorResponse(w, "source path does not exist")
		return
	}

	// Check if the destination path exists
	if _, err := os.Stat(absDestPath); !os.IsNotExist(err) {
		utils.SendErrorResponse(w, "destination path already exists")
		return
	}

	// Rename the source to the destination
	err = os.Rename(absSrcPath, absDestPath)
	if err != nil {
		utils.SendErrorResponse(w, fmt.Sprintf("error moving file/directory: %v", err))
		return
	}
	utils.SendOK(w)
}

func (fm *FileManager) HandleFileProperties(w http.ResponseWriter, r *http.Request) {
	// Parse the target file or directory path from the request
	filePath, err := utils.GetPara(r, "file")
	if err != nil {
		utils.SendErrorResponse(w, "invalid file path")
		return
	}

	// Construct the absolute path to the target file or directory
	absPath := filepath.Join(fm.Directory, filePath)

	// Check if the target path exists
	_, err = os.Stat(absPath)
	if err != nil {
		utils.SendErrorResponse(w, "file or directory does not exist")
		return
	}

	// Initialize a map to hold file properties
	fileProps := make(map[string]interface{})
	fileProps["filename"] = filepath.Base(absPath)
	fileProps["filepath"] = filePath
	fileProps["isDir"] = isDir(absPath)

	// Get file size and last modified time
	finfo, err := os.Stat(absPath)
	if err != nil {
		utils.SendErrorResponse(w, "unable to retrieve file properties")
		return
	}
	fileProps["lastModified"] = finfo.ModTime().Unix()
	if !isDir(absPath) {
		// If it's a file, get its size
		fileProps["size"] = finfo.Size()
	} else {
		// If it's a directory, calculate its total size containing all child files and folders
		totalSize, err := calculateDirectorySize(absPath)
		if err != nil {
			utils.SendErrorResponse(w, "unable to calculate directory size")
			return
		}
		fileProps["size"] = totalSize
	}

	// Count the number of sub-files and sub-folders
	numSubFiles, numSubFolders, err := countSubFilesAndFolders(absPath)
	if err != nil {
		utils.SendErrorResponse(w, "unable to count sub-files and sub-folders")
		return
	}
	fileProps["fileCounts"] = numSubFiles
	fileProps["folderCounts"] = numSubFolders

	// Serialize the file properties to JSON
	jsonData, err := json.Marshal(fileProps)
	if err != nil {
		utils.SendErrorResponse(w, "unable to marshal JSON")
		return
	}

	// Set response headers and send the JSON response
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write(jsonData)
}

// HandleFileDelete deletes a file or directory
func (fm *FileManager) HandleFileDelete(w http.ResponseWriter, r *http.Request) {
	// Parse the target file or directory path from the request
	filePath, err := utils.PostPara(r, "target")
	if err != nil {
		utils.SendErrorResponse(w, "invalid file path")
		return
	}

	// Construct the absolute path to the target file or directory
	absPath := filepath.Join(fm.Directory, filePath)

	// Check if the target path exists
	_, err = os.Stat(absPath)
	if err != nil {
		utils.SendErrorResponse(w, "file or directory does not exist")
		return
	}

	// Delete the file or directory
	err = os.RemoveAll(absPath)
	if err != nil {
		utils.SendErrorResponse(w, "error deleting file or directory")
		return
	}

	// Respond with a success message or appropriate response
	utils.SendOK(w)
}
