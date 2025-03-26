document.addEventListener('DOMContentLoaded', function() {
    // DOM elements
    const fileInput = document.getElementById('file-input');
    const fileLabel = document.querySelector('.file-label span');
    const uploadForm = document.getElementById('upload-form');
    const progressSection = document.getElementById('progress-section');
    const resultsSection = document.getElementById('results-section');
    const progressBar = document.getElementById('progress-bar');
    const progressPercentage = document.getElementById('progress-percentage');
    const successCount = document.getElementById('success-count');
    const failedCount = document.getElementById('failed-count');
    const processingCount = document.getElementById('processing-count');
    const cancelButton = document.getElementById('cancel-button');
    const successTab = document.getElementById('success-tab');
    const failureTab = document.getElementById('failure-tab');
    const successTable = document.getElementById('success-table');
    const failureTable = document.getElementById('failure-table');
    const successResults = document.getElementById('success-results');
    const failureResults = document.getElementById('failure-results');
    const loadingModal = document.getElementById('loading-modal');
    const downloadSuccessButton = document.getElementById('download-success');
    const downloadFailureButton = document.getElementById('download-failure');
    const viewResultsBtn = document.getElementById('view-results-button');

    // Application state
    let currentProcessId = null;
    let isProcessing = false;
    let pollingInterval = null;
    
    // Initialize theme
    initTheme();

    // Event listeners
    if (fileInput) {
        fileInput.addEventListener('change', handleFileChange);
    }
    
    if (uploadForm) {
        uploadForm.addEventListener('submit', handleStartProcess);
    }
    
    if (cancelButton) {
        cancelButton.addEventListener('click', cancelProcess);
    }
    
    if (successTab) {
        successTab.addEventListener('click', () => showTab('success'));
    }
    
    if (failureTab) {
        failureTab.addEventListener('click', () => showTab('failure'));
    }
    
    if (downloadSuccessButton) {
        downloadSuccessButton.addEventListener('click', () => downloadResults('success'));
    }
    
    if (downloadFailureButton) {
        downloadFailureButton.addEventListener('click', () => downloadResults('fail'));
    }

    // Initialize UI
    initUI();

    function initUI() {
        progressSection.style.display = 'none';
        resultsSection.style.display = 'none';
        successResults.style.display = 'block';
        failureResults.style.display = 'none';
        successTab.classList.add('active');
        loadingModal.style.display = 'none';
    }

    function initTheme() {
        const themeToggle = document.getElementById('theme-toggle');
        const savedTheme = localStorage.getItem('theme') || 'light';
        
        document.body.classList.add(savedTheme === 'dark' ? 'dark-mode' : 'light-mode');
        
        themeToggle.addEventListener('click', () => {
            const isDark = document.body.classList.contains('dark-mode');
            document.body.classList.toggle('dark-mode');
            document.body.classList.toggle('light-mode');
            localStorage.setItem('theme', isDark ? 'light' : 'dark');
        });
    }

    function handleFileChange(e) {
        const file = e.target.files[0];
        if (file) {
            fileLabel.textContent = file.name;
        } else {
            fileLabel.textContent = 'Chọn file Excel';
        }
    }

    async function handleStartProcess(e) {
        e.preventDefault();
        
        const file = fileInput.files[0];
        if (!file) {
            showNotification('Vui lòng chọn file Excel', 'error');
            return;
        }
        
        // Check file extension
        const extension = file.name.split('.').pop().toLowerCase();
        if (extension !== 'xlsx' && extension !== 'xls') {
            showNotification('Chỉ hỗ trợ file Excel (.xlsx hoặc .xls)', 'error');
            return;
        }
        
        // Show loading modal
        loadingModal.style.display = 'flex';
        
        try {
            // Upload file
            const formData = new FormData();
            formData.append('file', file);
            
            const uploadResponse = await fetch('/api/upload', {
                method: 'POST',
                body: formData
            });
            
            if (!uploadResponse.ok) {
                const errorText = await uploadResponse.text();
                throw new Error(errorText || 'Upload failed');
            }
            
            const uploadData = await uploadResponse.json();
            console.log('Upload response:', uploadData);
            
            // Start processing
            const workersValue = document.getElementById('threads').value;
            console.log('Workers value:', workersValue);
            
            const processResponse = await fetch('/api/process', {
                method: 'POST',
                headers: {
                    'Content-Type': 'application/json'
                },
                body: JSON.stringify({
                    filePath: uploadData.filePath,
                    workers: workersValue
                })
            });
            
            if (!processResponse.ok) {
                const errorText = await processResponse.text();
                throw new Error(errorText || 'Processing failed');
            }
            
            const processData = await processResponse.json();
            currentProcessId = processData.processId;
            
            // Hide loading modal
            loadingModal.style.display = 'none';
            
            // Show progress section and hide upload section
            document.getElementById('upload-section').style.display = 'none';
            progressSection.style.display = 'block';
            
            // Start polling for progress
            isProcessing = true;
            startPolling();
            
        } catch (error) {
            loadingModal.style.display = 'none';
            console.error('Error:', error);
            showNotification(error.message || 'Có lỗi xảy ra', 'error');
        }
    }

    function startPolling() {
        // Clear any existing interval
        if (pollingInterval) {
            clearInterval(pollingInterval);
        }
        
        // Poll every 2 seconds
        pollingInterval = setInterval(pollProgress, 2000);
        
        // Initial poll
        pollProgress();
    }

    async function pollProgress() {
        if (!currentProcessId || !isProcessing) {
            clearInterval(pollingInterval);
            return;
        }
        
        try {
            const response = await fetch(`/api/progress/${currentProcessId}`);
            
            if (!response.ok) {
                throw new Error('Failed to fetch progress');
            }
            
            const progressData = await response.json();
            updateUI(progressData);
            
            // If process is complete, stop polling
            if (progressData.isComplete) {
                clearInterval(pollingInterval);
                isProcessing = false;
                showResults(progressData);
            }
            
        } catch (error) {
            console.error('Error polling progress:', error);
            // Don't stop polling on error, just try again next time
        }
    }

    function updateUI(progressData) {
        // Update progress bar
        const progress = Math.round(progressData.progress);
        progressBar.style.width = `${progress}%`;
        progressPercentage.textContent = `${progress}%`;
        
        // Update counters
        successCount.textContent = progressData.successAccounts || 0;
        failedCount.textContent = progressData.failedAccounts || 0;
        processingCount.textContent = progressData.processingAccounts || 0;
    }

    function showResults(data) {
        // Enable results section
        resultsSection.style.display = 'block';
        
        // Populate tables
        populateTable(successTable, data.successData || []);
        populateTable(failureTable, data.failData || [], false);
        
        // Enable or disable download buttons based on data availability
        downloadSuccessButton.disabled = !(data.successData && data.successData.length > 0);
        downloadFailureButton.disabled = !(data.failData && data.failData.length > 0);
        
        // Hide loading spinner and cancel button when process is complete
        const loadingSpinner = document.querySelector('.loading-spinner');
        if (loadingSpinner) {
            loadingSpinner.style.display = 'none';
        }
        
        // Hide cancel button or change it to "New Process" button
        if (cancelButton) {
            cancelButton.textContent = 'Tiến trình mới';
            cancelButton.onclick = function() {
                // Reset UI to allow starting a new process
                document.getElementById('upload-section').style.display = 'block';
                progressSection.style.display = 'none';
                resultsSection.style.display = 'none';
                fileLabel.textContent = 'Chọn file Excel';
                fileInput.value = '';
                cancelButton.textContent = 'Hủy xử lý';
                cancelButton.onclick = cancelProcess;
                
                // Reset state
                currentProcessId = null;
                isProcessing = false;
            };
        }
    }

    function populateTable(tableIdOrElement, data) {
        // Get table element if ID was passed
        const table = typeof tableIdOrElement === 'string' 
            ? document.getElementById(tableIdOrElement) 
            : tableIdOrElement;
            
        if (!table) {
            console.error(`Table element not found: ${tableIdOrElement}`);
            return;
        }
        
        const tbody = table.querySelector('tbody') || table;
        tbody.innerHTML = ''; // Clear existing rows
        
        console.log(`Populating ${tableIdOrElement} with ${data.length} rows`);
        
        data.forEach((account, index) => {
            const row = document.createElement('tr');
            
            // Add row number
            const indexCell = document.createElement('td');
            indexCell.textContent = index + 1;
            row.appendChild(indexCell);
            
            // Add username/account
            const usernameCell = document.createElement('td');
            usernameCell.textContent = account.username || '';
            row.appendChild(usernameCell);
            
            const isSuccessTable = typeof tableIdOrElement === 'string' 
                ? tableIdOrElement === 'success-table' 
                : table.id === 'success-table';
                
            if (isSuccessTable) {
                // Add balance for success table
                const balanceCell = document.createElement('td');
                balanceCell.textContent = account.balance ? account.balance.toLocaleString() : '0';
                row.appendChild(balanceCell);
                
                // Add last deposit for success table
                const depositCell = document.createElement('td');
                depositCell.textContent = account.lastDeposit ? account.lastDeposit.toLocaleString() : '0';
                row.appendChild(depositCell);
                
                // Add deposit time for success table
                const timeCell = document.createElement('td');
                timeCell.textContent = account.depositTime || '';
                row.appendChild(timeCell);
            } else {
                // Add password for fail table
                const passwordCell = document.createElement('td');
                passwordCell.textContent = account.password || '';
                row.appendChild(passwordCell);
                
                // Add reason for fail table
                const reasonCell = document.createElement('td');
                reasonCell.textContent = account.reason || '';
                row.appendChild(reasonCell);
            }
            
            tbody.appendChild(row);
        });
        
        // Show table if it has data
        if (data.length > 0) {
            table.style.display = 'table';
            const tableId = typeof tableIdOrElement === 'string' ? tableIdOrElement : table.id;
            const emptyMessage = document.getElementById(
                tableId === 'success-table' || tableId === 'successTable' 
                    ? 'no-success-message' 
                    : 'no-fail-message'
            );
            if (emptyMessage) {
                emptyMessage.style.display = 'none';
            }
        } else {
            table.style.display = 'none';
            const tableId = typeof tableIdOrElement === 'string' ? tableIdOrElement : table.id;
            const emptyMessage = document.getElementById(
                tableId === 'success-table' || tableId === 'successTable' 
                    ? 'no-success-message' 
                    : 'no-fail-message'
            );
            if (emptyMessage) {
                emptyMessage.style.display = 'block';
            }
        }
    }

    async function cancelProcess() {
        if (!currentProcessId || !isProcessing) {
            return;
        }
        
        try {
            const response = await fetch(`/api/cancel/${currentProcessId}`, {
                method: 'POST'
            });
            
            if (!response.ok) {
                throw new Error('Failed to cancel process');
            }
            
            isProcessing = false;
            clearInterval(pollingInterval);
            showNotification('Đã hủy quá trình xử lý', 'info');
            
            // Reset UI to allow starting a new process
            document.getElementById('upload-section').style.display = 'block';
            fileLabel.textContent = 'Chọn file Excel';
            fileInput.value = '';
            
        } catch (error) {
            showNotification(error.message || 'Có lỗi xảy ra khi hủy', 'error');
        }
    }

    function showTab(tabName) {
        // Update tab visibility
        if (tabName === 'success') {
            successResults.style.display = 'block';
            failureResults.style.display = 'none';
            successTab.classList.add('active');
            failureTab.classList.remove('active');
        } else {
            successResults.style.display = 'none';
            failureResults.style.display = 'block';
            successTab.classList.remove('active');
            failureTab.classList.add('active');
        }
    }

    async function downloadResults(type) {
        window.location.href = `/api/download/${type}`;
    }

    function showNotification(message, type = 'info') {
        const notification = document.createElement('div');
        notification.className = `notification ${type}`;
        notification.textContent = message;
        
        document.body.appendChild(notification);
        
        // Fade in
        setTimeout(() => {
            notification.classList.add('show');
        }, 10);
        
        // Remove after 3 seconds
        setTimeout(() => {
            notification.classList.remove('show');
            setTimeout(() => {
                document.body.removeChild(notification);
            }, 300);
        }, 3000);
    }

    // Poll for process status
    function pollProcessStatus(processId) {
        if (isProcessing) {
            fetch(`/api/progress/${processId}`)
                .then(response => response.json())
                .then(data => {
                    updateProgressUI(data);
                    
                    // If process is complete
                    if (data.isComplete) {
                        isProcessing = false;
                        document.getElementById('progressSection').classList.add('complete');
                        document.getElementById('loadingModal').classList.remove('show');
                        
                        // Populate results
                        console.log("Process complete, populating results:", data);
                        if (data.successData && data.successData.length > 0) {
                            populateTable('successTable', data.successData);
                            document.getElementById('successCount').textContent = data.successData.length;
                        } else {
                            console.log("No success data available");
                        }
                        
                        if (data.failData && data.failData.length > 0) {
                            populateTable('failTable', data.failData);
                            document.getElementById('failCount').textContent = data.failData.length;
                        } else {
                            console.log("No fail data available");
                        }
                        
                        // Switch to results section
                        showSection('results');
                        
                        // Clear the polling interval
                        clearInterval(pollingInterval);
                        pollingInterval = null;
                    } else {
                        // Continue polling
                        if (!pollingInterval) {
                            pollingInterval = setInterval(() => pollProcessStatus(processId), 2000);
                        }
                    }
                })
                .catch(error => {
                    console.error('Error polling process status:', error);
                    // Don't stop polling on error, just try again
                });
        } else if (pollingInterval) {
            clearInterval(pollingInterval);
            pollingInterval = null;
        }
    }

    // Update the progress UI
    function updateProgressUI(data) {
        const progressBar = document.getElementById('progressBar');
        const progressText = document.getElementById('progressText');
        const successCount = document.getElementById('successCount');
        const failCount = document.getElementById('failCount');
        const processingCount = document.getElementById('processingCount');
        
        // Update progress bar
        progressBar.style.width = `${data.progress}%`;
        progressText.textContent = `${Math.round(data.progress)}%`;
        
        // Update counts
        successCount.textContent = data.successAccounts || 0;
        failCount.textContent = data.failedAccounts || 0;
        processingCount.textContent = data.processingAccounts || 0;
        
        // Update status message
        if (data.isComplete) {
            document.getElementById('statusMessage').textContent = 'Processing complete!';
            // Show the results link
            document.getElementById('viewResultsButton').style.display = 'block';
        } else {
            document.getElementById('statusMessage').textContent = 'Processing accounts...';
        }
    }
}); 