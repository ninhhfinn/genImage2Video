package main

// Xác thực module Dọn dẹp.
//
// Repo này trước đó KHÔNG có xác thực gì cả (công cụ video chạy nội bộ sau
// Cloudflare tunnel). Module Dọn dẹp thì bắt buộc phải có: nó chứa số điện thoại,
// ảnh trong nhà khách, và số tiền phải trả cho người thật.
//
// Một bảng người dùng, một cột `role` — không dựng hai hệ tài khoản song song.
// Quản lý và cô dọn dẹp đăng nhập cùng một cửa, backend quyết định thấy gì.

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
)

// Token sống 30 ngày: cô dọn dẹp dùng điện thoại cá nhân, bắt đăng nhập lại mỗi
// tuần là cách chắc chắn để họ bỏ tool. Quản lý cũng vậy — đây không phải ngân hàng.
const hkTokenTTL = 30 * 24 * time.Hour

var (
	errHKUnauthorized = errors.New("chưa đăng nhập")
	errHKForbidden    = errors.New("không có quyền")
)

func hkRandomID(prefix string) string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		// crypto/rand hỏng là chuyện của hệ điều hành; rơi về mốc thời gian còn
		// hơn trả id rỗng rồi ghi đè mất bản ghi khác.
		return fmt.Sprintf("%s_%d", prefix, time.Now().UnixNano())
	}
	return prefix + "_" + hex.EncodeToString(b)
}

func hkNewToken() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return ""
	}
	return hex.EncodeToString(b)
}

func hkHashPassword(pw string) (string, error) {
	h, err := bcrypt.GenerateFromPassword([]byte(pw), bcrypt.DefaultCost)
	return string(h), err
}

func hkCheckPassword(hash, pw string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(pw)) == nil
}

// hkTokenFromRequest đọc token theo thứ tự: header Authorization → header riêng
// → query param. Query param có mặt vì thẻ <img> không gửi được header, mà ảnh
// checklist thì phải bảo vệ.
func hkTokenFromRequest(r *http.Request) string {
	if v := r.Header.Get("Authorization"); strings.HasPrefix(v, "Bearer ") {
		return strings.TrimSpace(strings.TrimPrefix(v, "Bearer "))
	}
	if v := r.Header.Get("X-HK-Token"); v != "" {
		return strings.TrimSpace(v)
	}
	return strings.TrimSpace(r.URL.Query().Get("hk_token"))
}

// hkAuthUser trả người dùng đang đăng nhập, hoặc lỗi nếu token thiếu/hết hạn.
func (a *HKApp) hkAuthUser(r *http.Request) (HKUser, error) {
	token := hkTokenFromRequest(r)
	if token == "" {
		return HKUser{}, errHKUnauthorized
	}
	u, err := a.store.UserByToken(token, hkNowMs())
	if err != nil {
		return HKUser{}, errHKUnauthorized
	}
	// Bị khoá giữa chừng thì token cũ phải mất tác dụng ngay, không đợi hết hạn.
	if u.Status != HKStaffActive {
		return HKUser{}, errHKForbidden
	}
	return u, nil
}

func (a *HKApp) hkRequireAdmin(r *http.Request) (HKUser, error) {
	u, err := a.hkAuthUser(r)
	if err != nil {
		return u, err
	}
	if u.Role != HKRoleAdmin {
		return u, errHKForbidden
	}
	return u, nil
}

// hkSeedAdmin tạo tài khoản quản lý đầu tiên nếu DB chưa có admin nào.
//
// Mật khẩu lấy từ HK_ADMIN_PASSWORD; không đặt thì sinh ngẫu nhiên và IN RA
// CONSOLE một lần. Cố tình không có mật khẩu mặc định kiểu "admin123": web này
// nằm sau tên miền công khai, một mật khẩu mặc định đoán được là mở toang.
func hkSeedAdmin(store *HKStore) error {
	admins, err := store.ListUsers(HKRoleAdmin)
	if err != nil {
		return err
	}
	if len(admins) > 0 {
		return nil
	}

	phone := strings.TrimSpace(os.Getenv("HK_ADMIN_PHONE"))
	if phone == "" {
		phone = "0900000000"
	}
	pw := strings.TrimSpace(os.Getenv("HK_ADMIN_PASSWORD"))
	generated := false
	if pw == "" {
		pw = hkNewToken()[:12]
		generated = true
	}
	hash, err := hkHashPassword(pw)
	if err != nil {
		return err
	}
	u := HKUser{
		ID:           hkRandomID("hku"),
		Role:         HKRoleAdmin,
		Name:         "Quản lý",
		Phone:        phone,
		PasswordHash: hash,
		Status:       HKStaffActive,
		Zones:        []string{},
		CreatedAt:    hkNowMs(),
	}
	if err := store.UpsertUser(u); err != nil {
		return err
	}

	log.Printf("[hk] đã tạo tài khoản quản lý đầu tiên: %s", phone)
	if generated {
		fmt.Printf("\n════════════════════════════════════════════════════════\n")
		fmt.Printf("  TÀI KHOẢN QUẢN LÝ DỌN DẸP (chỉ hiện MỘT LẦN)\n")
		fmt.Printf("  Số điện thoại: %s\n", phone)
		fmt.Printf("  Mật khẩu:      %s\n", pw)
		fmt.Printf("  Đổi bằng biến môi trường HK_ADMIN_PASSWORD trước lần chạy đầu.\n")
		fmt.Printf("════════════════════════════════════════════════════════\n\n")
	}
	return nil
}
