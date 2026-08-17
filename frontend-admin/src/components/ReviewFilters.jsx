// Bộ lọc đánh giá — dùng chung cho cả màn quản lý và màn cô dọn dẹp.
//
// Hai vai lọc gần giống nhau (phòng, cơ sở, ngày tháng, mức sao) nên viết một
// lần; khác biệt duy nhất là quản lý có thêm ô lọc theo người dọn dẹp, truyền
// vào qua `extra`.
//
// Con số trên mỗi chip sao do BACKEND đếm trên tập đã lọc phòng/cơ sở/ngày nhưng
// CHƯA lọc sao — nhờ vậy bấm chip này không làm đổi số trên chip kia, và người
// dùng luôn biết còn bao nhiêu đơn 1 sao.

import { dayKey, shiftDay } from '../format.js'

const STARS = [5, 4, 3, 2, 1]

/** Khoảng ngày bấm nhanh — đỡ phải gõ tay hai ô ngày cho việc thường làm nhất. */
const PRESETS = [
	{ key: '7', label: '7 ngày', days: 7 },
	{ key: '30', label: '30 ngày', days: 30 },
	{ key: '90', label: '90 ngày', days: 90 },
]

export function ReviewFilters({ value, onChange, rooms = [], facilities = [], starCounts = {}, extra = null, compact = false }) {
	const set = (patch) => onChange({ ...value, ...patch })

	function toggleStar(n) {
		const cur = value.stars || []
		set({ stars: cur.includes(n) ? cur.filter((x) => x !== n) : [...cur, n] })
	}

	function applyPreset(days) {
		const to = dayKey()
		set({ from: shiftDay(to, -days + 1), to })
	}

	const activePreset = PRESETS.find(
		(p) => value.to === dayKey() && value.from === shiftDay(dayKey(), -p.days + 1),
	)?.key

	const hasFilter =
		value.room_id || value.facility_id || (value.stars || []).length || value.staff_id || activePreset !== '30'

	return (
		<div className={`filters${compact ? ' filters--compact' : ''}`}>
			<div className="filters-row">
				<label className="filter">
					<span>Phòng</span>
					<select value={value.room_id || ''} onChange={(e) => set({ room_id: e.target.value })}>
						<option value="">Tất cả phòng</option>
						{rooms.map((r) => (
							<option key={r.id} value={r.id}>
								{r.label}
							</option>
						))}
					</select>
				</label>

				<label className="filter">
					<span>Cơ sở</span>
					<select value={value.facility_id || ''} onChange={(e) => set({ facility_id: e.target.value })}>
						<option value="">Tất cả cơ sở</option>
						{facilities.map((f) => (
							<option key={f.id} value={f.id}>
								{f.label}
							</option>
						))}
					</select>
				</label>

				{extra}

				<label className="filter">
					<span>Từ ngày</span>
					<input type="date" value={value.from || ''} max={value.to || undefined} onChange={(e) => set({ from: e.target.value })} />
				</label>

				<label className="filter">
					<span>Đến ngày</span>
					<input type="date" value={value.to || ''} min={value.from || undefined} onChange={(e) => set({ to: e.target.value })} />
				</label>
			</div>

			<div className="filters-row filters-row--chips">
				{PRESETS.map((p) => (
					<button
						key={p.key}
						type="button"
						className={`chip chip--range${activePreset === p.key ? ' active' : ''}`}
						onClick={() => applyPreset(p.days)}
					>
						{p.label}
					</button>
				))}

				<span className="filters-sep" aria-hidden="true" />

				{STARS.map((n) => {
					const count = starCounts[String(n)] || 0
					const on = (value.stars || []).includes(n)
					return (
						<button
							key={n}
							type="button"
							className={`chip chip--star chip--star${n}${on ? ' active' : ''}`}
							// Vẫn cho bấm chip 0 đánh giá: người dùng bấm để KIỂM TRA xem có
							// đơn 1 sao nào không, và nhận về "không có" là câu trả lời đúng.
							onClick={() => toggleStar(n)}
							title={`${n} sao — ${count} đánh giá`}
						>
							{n}★ <em>{count}</em>
						</button>
					)
				})}

				{hasFilter && (
					<button
						type="button"
						className="chip chip--clear"
						onClick={() => onChange({ room_id: '', facility_id: '', staff_id: '', stars: [], from: shiftDay(dayKey(), -29), to: dayKey() })}
					>
						Xoá lọc
					</button>
				)}
			</div>
		</div>
	)
}

/** Bộ lọc mặc định: 30 ngày gần nhất, không lọc gì khác. */
export function defaultReviewFilter() {
	const to = dayKey()
	return { room_id: '', facility_id: '', staff_id: '', stars: [], from: shiftDay(to, -29), to }
}

/** Đổi bộ lọc thành query string cho API. */
export function reviewQuery(f) {
	const q = new URLSearchParams()
	if (f.room_id) q.set('room_id', f.room_id)
	if (f.facility_id) q.set('facility_id', f.facility_id)
	if (f.staff_id) q.set('staff_id', f.staff_id)
	if ((f.stars || []).length) q.set('stars', f.stars.join(','))
	if (f.from) q.set('from', f.from)
	if (f.to) q.set('to', f.to)
	return q.toString()
}
