// Lớp gọi API duy nhất của app. Không component nào gọi fetch trực tiếp.
//
// Hai việc file này làm mà chỗ khác không phải nghĩ tới nữa:
//   1. Gắn token vào mọi request (kể cả URL ảnh, vì thẻ <img> không gửi header).
//   2. Bóc lỗi thành CHUỖI TIẾNG VIỆT sẵn sàng hiện lên màn — backend đã viết
//      câu cho người dùng cuối đọc, nên chỗ gọi chỉ việc hiển thị.

const TOKEN_KEY = 'hk-token'

export function getToken() {
	try {
		return localStorage.getItem(TOKEN_KEY) || ''
	} catch {
		return ''
	}
}

export function setToken(token) {
	try {
		if (token) localStorage.setItem(TOKEN_KEY, token)
		else localStorage.removeItem(TOKEN_KEY)
	} catch {
		/* chế độ riêng tư của Safari chặn localStorage — vẫn chạy được trong phiên */
	}
}

/** URL ảnh kèm token, dùng cho <img src>. */
export function photoSrc(url) {
	if (!url) return ''
	const t = getToken()
	return t ? `${url}?hk_token=${encodeURIComponent(t)}` : url
}

class ApiError extends Error {
	constructor(message, status) {
		super(message)
		this.status = status
	}
}

async function request(path, { method = 'GET', body, formData } = {}) {
	const headers = {}
	const token = getToken()
	if (token) headers['X-HK-Token'] = token

	let payload
	if (formData) {
		payload = formData
	} else if (body !== undefined) {
		headers['Content-Type'] = 'application/json'
		payload = JSON.stringify(body)
	}

	let res
	try {
		res = await fetch(path, { method, headers, body: payload })
	} catch {
		// Mất mạng giữa chừng là chuyện thường khi cô đứng trong nhà bê tông.
		throw new ApiError('Mất kết nối mạng. Kiểm tra sóng rồi thử lại nhé.', 0)
	}

	const text = await res.text()
	let data = null
	if (text) {
		try {
			data = JSON.parse(text)
		} catch {
			data = null
		}
	}

	if (!res.ok) {
		const msg = (data && data.error) || 'Có lỗi xảy ra, thử lại sau nhé.'
		if (res.status === 401) setToken('')
		throw new ApiError(msg, res.status)
	}
	return data
}

const get = (p) => request(p)
const post = (p, body) => request(p, { method: 'POST', body })

export const api = {
	login: (phone, password) => post('/api/hk/login', { phone, password }),
	register: (payload) => post('/api/hk/register', payload),
	logout: () => post('/api/hk/logout', {}),
	me: () => get('/api/hk/me'),
	meta: () => get('/api/hk/meta'),

	staffs: () => get('/api/hk/staffs'),
	reviewStaff: (id, status) => post('/api/hk/staffs/review', { id, status }),

	rooms: () => get('/api/hk/rooms'),
	syncRooms: (limit) => post('/api/hk/rooms/sync', { limit }),
	saveRoom: (payload) => post('/api/hk/rooms/settings', payload),

	templates: () => get('/api/hk/templates'),
	saveTemplate: (tpl) => post('/api/hk/templates', tpl),
	applyTemplate: (templateId, roomIds) => post('/api/hk/templates/apply', { template_id: templateId, room_ids: roomIds }),

	sessions: (filter = {}) => {
		const q = new URLSearchParams()
		Object.entries(filter).forEach(([k, v]) => {
			if (v !== '' && v != null) q.set(k, v)
		})
		const s = q.toString()
		return get('/api/hk/sessions' + (s ? '?' + s : ''))
	},
	session: (id) => get('/api/hk/sessions/get?id=' + encodeURIComponent(id)),
	syncSessions: (ahead) => post('/api/hk/sessions/sync', { ahead }),
	startSession: (id) => post('/api/hk/sessions/start', { id }),
	saveItem: (id, itemId, patch) => post('/api/hk/sessions/item', { id, item_id: itemId, ...patch }),
	submitSession: (id) => post('/api/hk/sessions/submit', { id }),
	reportIssue: (id, note) => post('/api/hk/sessions/note', { id, note }),

	issues: (query = '') => get('/api/hk/issues' + (query ? '?' + query : '')),
	createIssue: (payload) => post('/api/hk/issues/create', payload),
	claimIssue: (id, deadlineAt) => post('/api/hk/issues/claim', { id, deadline_at: deadlineAt }),
	assignIssue: (payload) => post('/api/hk/issues/assign', payload),
	resolveIssue: (payload) => post('/api/hk/issues/resolve', payload),
	setStaffRole: (id, role) => post('/api/hk/staffs/role', { id, role }),
	assignSession: (id, staffId) => post('/api/hk/sessions/assign', { id, staff_id: staffId }),
	reviewSession: (payload) => post('/api/hk/sessions/review', payload),

	report: ({ day, month }) => {
		const q = day ? 'day=' + encodeURIComponent(day) : 'month=' + encodeURIComponent(month)
		return get('/api/hk/report?' + q)
	},
	reviews: (query = '') => get('/api/hk/reviews' + (query ? '?' + query : '')),
	revenue: (query = '') => get('/api/hk/revenue' + (query ? '?' + query : '')),
	syncRevenue: (days) => post('/api/hk/revenue/sync', { days }),
	syncReviews: (days) => post('/api/hk/reviews/sync', { days }),

	uploadPhoto: (file) => {
		const fd = new FormData()
		fd.append('photo', file)
		return request('/api/hk/photos', { method: 'POST', formData: fd })
	},
}
