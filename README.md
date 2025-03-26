# K2 Check Web Interface

Giao diện web cho công cụ K2 Check với thiết kế Neumorphism, hỗ trợ xử lý tài khoản hàng loạt.

## Tính năng

- Tải lên file Excel chứa danh sách tài khoản
- Xử lý đăng nhập hàng loạt với nhiều luồng
- Giải captcha tự động
- Theo dõi tiến trình xử lý theo thời gian thực
- Hiển thị kết quả đăng nhập thành công/thất bại
- Xuất kết quả ra file Excel
- Hỗ trợ chế độ sáng/tối
- Thiết kế responsive cho cả desktop và mobile

## Cài đặt

### Yêu cầu

- Go 1.16+
- Gorilla Mux (`go get -u github.com/gorilla/mux`)

### Cài đặt và chạy

1. Clone repository này:
```
git clone https://github.com/khahanv2/smart-code-project.git
cd smart-code-project/web
```

2. Cài đặt dependencies:
```
go mod download
```

3. Chạy server:
```
go run server.go
```

4. Mở trình duyệt và truy cập:
```
http://localhost:8080
```

## Cách sử dụng

1. Chọn file Excel (.xlsx hoặc .xls) chứa danh sách tài khoản
2. Chọn số luồng xử lý (mặc định: 2)
3. Nhấn "Bắt đầu xử lý"
4. Theo dõi tiến trình xử lý
5. Xem kết quả và tải xuống nếu cần thiết

## Cấu trúc thư mục

```
web/
├── css/               # CSS styles
│   └── style.css      # Main stylesheet
├── js/                # JavaScript files
│   └── script.js      # Main script file
├── img/               # Images and icons
├── uploads/           # Temporary storage for uploaded files
├── results/           # Storage for result files
├── index.html         # Main HTML file
├── server.go          # Go API server
├── go.mod             # Go module file
└── README.md          # This file
```

## Lưu ý quan trọng

Đảm bảo rằng các thư mục sau tồn tại và có quyền ghi:
- `uploads/` - nơi lưu trữ tạm thời các file Excel được tải lên
- `results/` - nơi lưu trữ các file kết quả

Đảm bảo rằng binary `batch_login` nằm trong thư mục cha (`../batch_login`) 