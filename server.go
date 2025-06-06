package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/mux"
	"github.com/xuri/excelize/v2"
)

// Global variables
var (
	tempDir        = "./uploads"
	resultsDir     = "../results"
	processes      = make(map[string]*Process)
	processesMutex sync.Mutex
)

// Process represents a running batch_login process
type Process struct {
	ID              string
	Cmd             *exec.Cmd
	IsComplete      bool
	TotalAccounts   int
	SuccessAccounts int
	FailedAccounts  int
	SuccessFilePath string
	FailFilePath    string
	StartTime       time.Time
	EndTime         time.Time
	UploadFilePath  string
	Status          string
	Threads         int
}

// ProcessProgress represents the progress of a batch_login process
type ProcessProgress struct {
	Progress           float64   `json:"progress"`
	TotalAccounts      int       `json:"totalAccounts"`
	SuccessAccounts    int       `json:"successAccounts"`
	FailedAccounts     int       `json:"failedAccounts"`
	ProcessingAccounts int       `json:"processingAccounts"`
	IsComplete         bool      `json:"isComplete"`
	SuccessData        []Account `json:"successData,omitempty"`
	FailData           []Account `json:"failData,omitempty"`
}

// Account represents an account in the results
type Account struct {
	Username      string  `json:"username"`
	Password      string  `json:"password,omitempty"`
	Success       bool    `json:"success"`
	Balance       float64 `json:"balance,omitempty"`
	LastDeposit   float64 `json:"lastDeposit,omitempty"`
	DepositTime   string  `json:"depositTime,omitempty"`
	DepositTxCode string  `json:"depositTxCode,omitempty"`
	Reason        string  `json:"reason,omitempty"`
}

func main() {
	// Create required directories
	os.MkdirAll(tempDir, 0755)
	os.MkdirAll(resultsDir, 0755)

	// Create router
	r := mux.NewRouter()

	// Serve static files
	r.PathPrefix("/css/").Handler(http.StripPrefix("/css/", http.FileServer(http.Dir("./css"))))
	r.PathPrefix("/js/").Handler(http.StripPrefix("/js/", http.FileServer(http.Dir("./js"))))
	r.PathPrefix("/img/").Handler(http.StripPrefix("/img/", http.FileServer(http.Dir("./img"))))

	// API routes
	r.HandleFunc("/api/upload", handleFileUpload).Methods("POST")
	r.HandleFunc("/api/process", handleStartProcess).Methods("POST")
	r.HandleFunc("/api/progress/{id}", handleGetProgress).Methods("GET")
	r.HandleFunc("/api/cancel/{id}", handleCancelProcess).Methods("POST")
	r.HandleFunc("/api/download/{type}", handleDownloadResults).Methods("GET")

	// Serve index.html for the root route
	r.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "index.html")
	})

	// Start server
	port := 8080
	log.Printf("Server started on http://localhost:%d\n", port)
	log.Printf("Uploads directory: %s\n", tempDir)
	log.Printf("Results directory: %s\n", resultsDir)
	log.Fatal(http.ListenAndServe(fmt.Sprintf(":%d", port), r))
}

func handleFileUpload(w http.ResponseWriter, r *http.Request) {
	// Parse the multipart form
	err := r.ParseMultipartForm(10 << 20) // 10 MB max
	if err != nil {
		http.Error(w, "Could not parse form", http.StatusBadRequest)
		return
	}

	// Get the file from the form
	file, handler, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "Error retrieving file", http.StatusBadRequest)
		return
	}
	defer file.Close()

	// Check if it's an Excel file
	if !strings.HasSuffix(handler.Filename, ".xlsx") && !strings.HasSuffix(handler.Filename, ".xls") {
		http.Error(w, "Only Excel files (.xlsx or .xls) are allowed", http.StatusBadRequest)
		return
	}

	// Create a unique filename
	timestamp := time.Now().Format("20060102_150405")
	filename := fmt.Sprintf("%s_%s", timestamp, handler.Filename)
	filePath := filepath.Join(tempDir, filename)

	// Get absolute path for consistent checking
	absFilePath, err := filepath.Abs(filePath)
	if err != nil {
		http.Error(w, "Could not determine absolute path", http.StatusInternalServerError)
		return
	}

	// Create the file
	dst, err := os.Create(filePath)
	if err != nil {
		http.Error(w, fmt.Sprintf("Could not create file: %v", err), http.StatusInternalServerError)
		return
	}
	defer dst.Close()

	// Copy the uploaded file to the destination file
	_, err = io.Copy(dst, file)
	if err != nil {
		http.Error(w, "Could not save file", http.StatusInternalServerError)
		return
	}

	log.Printf("File uploaded successfully: %s", absFilePath)

	// Respond with the file path
	response := map[string]string{
		"filePath": absFilePath,
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func handleStartProcess(w http.ResponseWriter, r *http.Request) {
	// Parse the request body
	var request struct {
		FilePath string `json:"filePath"`
		Workers  string `json:"workers"`
	}

	err := json.NewDecoder(r.Body).Decode(&request)
	if err != nil {
		http.Error(w, fmt.Sprintf("Invalid request body: %v", err), http.StatusBadRequest)
		return
	}

	log.Printf("Received process request - FilePath: %s, Workers: %s", request.FilePath, request.Workers)

	// Check if file exists
	if _, err := os.Stat(request.FilePath); os.IsNotExist(err) {
		http.Error(w, fmt.Sprintf("File does not exist: %s", request.FilePath), http.StatusBadRequest)
		return
	}

	// Validate file path - make sure it's an Excel file in our uploads directory
	absFilePath, err := filepath.Abs(request.FilePath)
	if err != nil {
		http.Error(w, "Could not determine absolute path", http.StatusInternalServerError)
		return
	}

	absTempDir, err := filepath.Abs(tempDir)
	if err != nil {
		http.Error(w, "Could not determine absolute temp directory path", http.StatusInternalServerError)
		return
	}

	if !strings.HasPrefix(absFilePath, absTempDir) && !strings.HasPrefix(request.FilePath, tempDir) {
		log.Printf("Invalid file path: %s not in %s", absFilePath, absTempDir)
		http.Error(w, "Invalid file path", http.StatusBadRequest)
		return
	}

	// Validate number of workers
	workers, err := strconv.Atoi(request.Workers)
	if err != nil || workers < 1 {
		workers = 2 // Default to 2 workers
	}

	// Create a unique ID for this process
	processID := fmt.Sprintf("process_%s", time.Now().Format("20060102_150405"))

	// Set up the batch_login command
	cmd := exec.Command("../batch_login", request.FilePath, strconv.Itoa(workers))

	log.Printf("Starting batch_login with command: %s %s %s", "../batch_login", request.FilePath, strconv.Itoa(workers))

	// Start the process
	err = cmd.Start()
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to start process: %v", err), http.StatusInternalServerError)
		return
	}

	// Đếm số tài khoản trong file Excel
	totalAccounts, err := countAccountsInExcel(request.FilePath)
	if err != nil {
		log.Printf("Lỗi khi đếm tài khoản: %v", err)
		totalAccounts = 0 // Mặc định nếu có lỗi
	}

	// Store the process information
	processesMutex.Lock()
	processes[processID] = &Process{
		ID:              processID,
		Cmd:             cmd,
		IsComplete:      false,
		TotalAccounts:   totalAccounts, // Sử dụng giá trị đếm được
		SuccessAccounts: 0,
		FailedAccounts:  0,
		StartTime:       time.Now(),
		UploadFilePath:  request.FilePath,
		Status:          "running",
		Threads:         workers,
	}
	processesMutex.Unlock()

	// Start a goroutine to wait for the process to complete
	go func() {
		err := cmd.Wait()
		if err != nil {
			log.Printf("Process %s completed with error: %v", processID, err)
		} else {
			log.Printf("Process %s completed successfully", processID)
		}

		processesMutex.Lock()
		defer processesMutex.Unlock()

		if process, ok := processes[processID]; ok {
			process.IsComplete = true
			process.EndTime = time.Now()

			// Find the result files - search with a broader pattern
			log.Printf("Searching for result files in: %s", resultsDir)
			resultFiles, err := filepath.Glob(filepath.Join(resultsDir, "*.xlsx"))
			if err != nil {
				log.Printf("Error searching for result files: %v", err)
			}

			log.Printf("Found %d possible result files", len(resultFiles))
			for _, file := range resultFiles {
				log.Printf("Checking result file: %s", file)
				if strings.Contains(file, "success") {
					process.SuccessFilePath = file
					log.Printf("Found success file: %s", file)
				} else if strings.Contains(file, "fail") {
					process.FailFilePath = file
					log.Printf("Found fail file: %s", file)
				}
			}

			// If no files found, try looking in the current working directory
			if process.SuccessFilePath == "" && process.FailFilePath == "" {
				log.Printf("No result files found in %s, trying current directory", resultsDir)
				// Get current working directory
				cwd, err := os.Getwd()
				if err == nil {
					resultFiles, _ = filepath.Glob(filepath.Join(cwd, "*.xlsx"))
					log.Printf("Found %d possible result files in current directory", len(resultFiles))
					for _, file := range resultFiles {
						log.Printf("Checking result file in cwd: %s", file)
						if strings.Contains(file, "success") {
							process.SuccessFilePath = file
							log.Printf("Found success file in cwd: %s", file)
						} else if strings.Contains(file, "fail") {
							process.FailFilePath = file
							log.Printf("Found fail file in cwd: %s", file)
						}
					}
				}
			}

			// Final check
			if process.SuccessFilePath == "" && process.FailFilePath == "" {
				log.Printf("Warning: No result files found after process completion")
			}

			// Read the Excel file to count the total number of accounts
			xlFile, err := excelize.OpenFile(request.FilePath)
			if err != nil {
				log.Printf("Lỗi khi mở file Excel: %v", err)
				return
			}

			// Đếm số dòng trong sheet đầu tiên
			sheetName := xlFile.GetSheetName(0)
			rows, err := xlFile.GetRows(sheetName)
			if err != nil {
				log.Printf("Lỗi khi đọc dữ liệu từ Excel: %v", err)
				return
			}

			// Đếm số dòng dữ liệu (trừ header nếu có)
			process.TotalAccounts = len(rows)
			if process.TotalAccounts > 0 {
				process.TotalAccounts-- // Trừ dòng header
			}
		}
	}()

	// Respond with the process ID and total accounts
	log.Printf("Started process %s with %d accounts", processID, totalAccounts)
	response := map[string]interface{}{
		"processId":     processID,
		"totalAccounts": totalAccounts,
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func handleGetProgress(w http.ResponseWriter, r *http.Request) {
	// Get the process ID from the URL
	vars := mux.Vars(r)
	processID := vars["id"]

	// Check if the process exists
	processesMutex.Lock()
	process, ok := processes[processID]
	processesMutex.Unlock()

	if !ok {
		http.Error(w, "Process not found", http.StatusNotFound)
		return
	}

	// Create the response
	progress := &ProcessProgress{
		TotalAccounts:      process.TotalAccounts, // Lấy trực tiếp từ process
		SuccessAccounts:    0,
		FailedAccounts:     0,
		ProcessingAccounts: process.TotalAccounts, // Mặc định tất cả đang xử lý
		IsComplete:         process.IsComplete,
	}

	// If the process is complete, read the result files
	if process.IsComplete {
		log.Printf("Process %s is complete, reading result files", processID)

		// Read success file if available
		if process.SuccessFilePath != "" {
			log.Printf("Reading success file: %s", process.SuccessFilePath)
			successAccounts, err := readExcelResults(process.SuccessFilePath, true)
			if err != nil {
				log.Printf("Error reading success file: %v", err)
			} else {
				progress.SuccessAccounts = len(successAccounts)
				progress.SuccessData = successAccounts
				log.Printf("Read %d success accounts", len(successAccounts))
			}
		} else {
			log.Printf("No success file found for process %s", processID)
		}

		// Read fail file if available
		if process.FailFilePath != "" {
			log.Printf("Reading fail file: %s", process.FailFilePath)
			failAccounts, err := readExcelResults(process.FailFilePath, false)
			if err != nil {
				log.Printf("Error reading fail file: %v", err)
			} else {
				progress.FailedAccounts = len(failAccounts)
				progress.FailData = failAccounts
				log.Printf("Read %d failed accounts", len(failAccounts))
			}
		} else {
			log.Printf("No fail file found for process %s", processID)
		}

		// Cập nhật số lượng tài khoản đang xử lý và tiến trình
		progress.ProcessingAccounts = progress.TotalAccounts - progress.SuccessAccounts - progress.FailedAccounts
		if progress.ProcessingAccounts < 0 {
			progress.ProcessingAccounts = 0
		}

		// Tiến trình hoàn thành
		progress.Progress = 100.0
	} else {
		// Đảm bảo dùng giá trị tài khoản thực tế từ tệp Excel
		progress.TotalAccounts = process.TotalAccounts

		// Nếu chưa có kết quả, tất cả tài khoản đều đang xử lý
		progress.ProcessingAccounts = progress.TotalAccounts - progress.SuccessAccounts - progress.FailedAccounts
		if progress.ProcessingAccounts < 0 {
			progress.ProcessingAccounts = 0
		}

		// Tính tỉ lệ phần trăm dựa trên số tài khoản đã xử lý xong
		if progress.TotalAccounts > 0 {
			estimatedProgress := float64(progress.SuccessAccounts+progress.FailedAccounts) / float64(progress.TotalAccounts) * 100.0
			if estimatedProgress > 99.0 {
				estimatedProgress = 99.0
			}
			progress.Progress = estimatedProgress
		} else {
			progress.Progress = 0
		}

		// Kiểm tra nếu vẫn chưa có tổng số tài khoản (do lỗi đọc file), thử đọc lại
		if progress.TotalAccounts == 0 {
			// Thử đọc số tài khoản từ file một lần nữa
			if totalAccounts, err := countAccountsInExcel(process.UploadFilePath); err == nil && totalAccounts > 0 {
				progress.TotalAccounts = totalAccounts
				progress.ProcessingAccounts = totalAccounts - progress.SuccessAccounts - progress.FailedAccounts
			}
		}
	}

	// Send the response
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(progress)
}

func handleCancelProcess(w http.ResponseWriter, r *http.Request) {
	// Get the process ID from the URL
	vars := mux.Vars(r)
	processID := vars["id"]

	// Check if the process exists
	processesMutex.Lock()
	process, ok := processes[processID]
	processesMutex.Unlock()

	if !ok {
		http.Error(w, "Process not found", http.StatusNotFound)
		return
	}

	// Kill the process
	if process.Cmd != nil && process.Cmd.Process != nil {
		err := process.Cmd.Process.Kill()
		if err != nil {
			http.Error(w, fmt.Sprintf("Failed to kill process: %v", err), http.StatusInternalServerError)
			return
		}
	}

	// Mark the process as complete
	processesMutex.Lock()
	process.IsComplete = true
	processesMutex.Unlock()

	// Send a success response
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"status":"ok"}`))
}

func handleDownloadResults(w http.ResponseWriter, r *http.Request) {
	// Get the type from the URL
	vars := mux.Vars(r)
	resultType := vars["type"]

	if resultType != "success" && resultType != "fail" {
		http.Error(w, "Invalid result type", http.StatusBadRequest)
		return
	}

	// Find the most recent result file of the specified type
	pattern := fmt.Sprintf("%s_*", resultType)
	files, err := filepath.Glob(filepath.Join(resultsDir, pattern))
	if err != nil || len(files) == 0 {
		http.Error(w, "No result file found", http.StatusNotFound)
		return
	}

	// Sort files by modification time (newest first)
	var latestFile string
	var latestTime time.Time
	for _, file := range files {
		info, err := os.Stat(file)
		if err != nil {
			continue
		}
		if latestFile == "" || info.ModTime().After(latestTime) {
			latestFile = file
			latestTime = info.ModTime()
		}
	}

	// Set the appropriate headers
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%s", filepath.Base(latestFile)))
	w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")

	// Send the file
	http.ServeFile(w, r, latestFile)
}

// readExcelResults reads the Excel results file and returns the accounts
func readExcelResults(filePath string, isSuccess bool) ([]Account, error) {
	log.Printf("Reading Excel file: %s", filePath)

	// Check if file exists
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		log.Printf("File does not exist: %s", filePath)
		return nil, fmt.Errorf("file does not exist: %s", filePath)
	}

	// Open the Excel file
	f, err := excelize.OpenFile(filePath)
	if err != nil {
		log.Printf("Error opening Excel file: %v", err)
		return nil, err
	}
	defer f.Close()

	// Get the first sheet name
	sheets := f.GetSheetList()
	if len(sheets) == 0 {
		log.Printf("No sheets found in Excel file")
		return nil, fmt.Errorf("no sheets found in Excel file")
	}

	// Get all rows from the first sheet
	rows, err := f.GetRows(sheets[0])
	if err != nil {
		log.Printf("Error reading rows: %v", err)
		return nil, err
	}

	// Skip header row if present
	dataStartRow := 0
	if len(rows) > 0 {
		dataStartRow = 1
	}

	var accounts []Account

	// Process rows based on whether it's a success or fail file
	if isSuccess {
		for i := dataStartRow; i < len(rows); i++ {
			row := rows[i]
			if len(row) < 4 {
				continue // Skip incomplete rows
			}

			// Extract data from columns
			username := row[0]
			balance, _ := strconv.ParseFloat(strings.Replace(row[1], ",", "", -1), 64)
			lastDeposit, _ := strconv.ParseFloat(strings.Replace(row[2], ",", "", -1), 64)
			depositTime := row[3]

			account := Account{
				Username:    username,
				Success:     true,
				Balance:     balance,
				LastDeposit: lastDeposit,
				DepositTime: depositTime,
			}
			accounts = append(accounts, account)
		}
	} else {
		for i := dataStartRow; i < len(rows); i++ {
			row := rows[i]
			if len(row) < 3 {
				continue // Skip incomplete rows
			}

			// Extract data from columns
			username := row[0]
			password := row[1]
			reason := row[2]

			account := Account{
				Username: username,
				Password: password,
				Success:  false,
				Reason:   reason,
			}
			accounts = append(accounts, account)
		}
	}

	log.Printf("Read %d accounts from %s", len(accounts), filePath)
	return accounts, nil
}

// Sửa hàm đọc file Excel để đếm đúng số tài khoản (chỉ đếm dòng có cả username và password)
func countAccountsInExcel(filePath string) (int, error) {
	// Mở file Excel
	f, err := excelize.OpenFile(filePath)
	if err != nil {
		log.Printf("Lỗi khi mở file Excel: %v", err)
		return 0, err
	}
	defer f.Close()

	// Lấy tên sheet đầu tiên
	sheetName := f.GetSheetName(0)
	rows, err := f.GetRows(sheetName)
	if err != nil {
		log.Printf("Lỗi khi đọc dữ liệu từ Excel: %v", err)
		return 0, err
	}

	// Bỏ qua dòng header
	accountCount := 0
	for i := 1; i < len(rows); i++ { // Bắt đầu từ dòng 1 (sau header)
		row := rows[i]
		// Kiểm tra nếu row có đủ 3 cột và cột 2 (username) và cột 3 (password) không rỗng
		if len(row) >= 3 && row[1] != "" && row[2] != "" {
			accountCount++
		}
	}

	log.Printf("Đếm được %d tài khoản hợp lệ trong file Excel", accountCount)
	return accountCount, nil
}
