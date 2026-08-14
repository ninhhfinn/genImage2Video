import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

// App quản lý dọn dẹp — admin.quanlyhomestay.com.
//
// Cổng dev 5174 để chạy song song với công cụ video (5173): sửa app này không
// phải tắt app kia. Build ra ../backend/dist-admin, cùng một binary Go phục vụ
// cả hai và tách nhau bằng tên miền (xem isAdminHost trong backend/server.go).
export default defineConfig({
  plugins: [react()],
  server: {
    port: 5174,
    proxy: {
      '/api': { target: 'http://localhost:8080', changeOrigin: true }
    }
  },
  build: {
    outDir: '../backend/dist-admin',
    emptyOutDir: true,
    // Thư mục tài nguyên riêng, KHÔNG dùng 'assets' mặc định.
    //
    // Hai app dùng chung một tên miền khi chạy local (/admin cho app này, / cho
    // công cụ video). index.html của Vite trỏ tài nguyên bằng đường dẫn tuyệt đối
    // `/assets/…`, nên nếu cả hai cùng tên thì request JS của app quản lý rơi vào
    // dist của app video, không tìm thấy, SPA fallback trả index.html — trình
    // duyệt nhận HTML thay vì JS và hiện trang trắng.
    //
    // Tên riêng làm request tự phân biệt được, đúng cả khi chạy dưới /admin lẫn
    // khi chạy ở gốc tên miền admin.quanlyhomestay.com.
    assetsDir: 'assets-admin',
  }
})
