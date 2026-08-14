// Định dạng hiển thị. KHÔNG có phép tính tiền ở đây — backend tính, frontend hiện.

export function money(n) {
	return (Math.round(Number(n) || 0)).toLocaleString('vi-VN') + 'đ'
}

export function count(n) {
	return (Number(n) || 0).toLocaleString('vi-VN')
}

const pad = (x) => String(x).padStart(2, '0')

export function hour(ms) {
	if (!ms) return '—'
	const d = new Date(Number(ms))
	return `${pad(d.getHours())}:${pad(d.getMinutes())}`
}

export function dayMonth(ms) {
	if (!ms) return '—'
	const d = new Date(Number(ms))
	return `${pad(d.getDate())}/${pad(d.getMonth() + 1)}`
}

export function dayKey(date = new Date()) {
	return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())}`
}

export function monthKey(date = new Date()) {
	return `${date.getFullYear()}-${pad(date.getMonth() + 1)}`
}

export function shiftDay(key, delta) {
	const [y, m, d] = key.split('-').map(Number)
	return dayKey(new Date(y, m - 1, d + delta))
}

export function shiftMonth(key, delta) {
	const [y, m] = key.split('-').map(Number)
	return monthKey(new Date(y, m - 1 + delta, 1))
}

export function monthLabel(key) {
	const [y, m] = String(key || '').split('-')
	if (!m) return key || '—'
	return `Tháng ${Number(m)}/${y}`
}

export function dayLabel(key, today = dayKey()) {
	if (key === today) return 'Hôm nay'
	if (key === shiftDay(today, -1)) return 'Hôm qua'
	if (key === shiftDay(today, 1)) return 'Ngày mai'
	const [y, m, d] = key.split('-')
	return `${d}/${m}/${y}`
}

export const SESSION_LABEL = {
	todo: 'Chưa bắt đầu',
	in_progress: 'Đang dọn',
	// "Chờ đối soát" = công ĐÃ được ghi nhận, đang chờ quản lý xác nhận — không
	// phải "chưa được gì". Diễn đạt sai chỗ này là cô gọi điện hỏi vì sao làm rồi
	// mà không có công.
	submitted: 'Chờ đối soát',
	approved: 'Đã duyệt',
	rejected: 'Bị từ chối',
}

export const STAFF_LABEL = {
	pending: 'Chờ duyệt',
	active: 'Đang làm',
	suspended: 'Tạm khoá',
	rejected: 'Từ chối',
}

export const ALLOWANCE_LABEL = {
	pending: 'Chờ duyệt',
	approved: 'Đã duyệt',
	rejected: 'Không duyệt',
}

export const ROOM_TYPE_LABEL = {
	studio: 'Studio',
	one_bedroom: '1 phòng ngủ',
	two_bedroom: '2 phòng ngủ',
	duplex: 'Duplex / nhiều tầng',
}
