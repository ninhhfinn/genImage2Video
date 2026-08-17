// Các màn của quản lý: điều phối ca, đối soát công, bảng công, nhân sự, phòng,
// mẫu checklist.

import { Fragment, useEffect, useMemo, useState } from 'react'
import { api, photoSrc } from '../api.js'
import { useAuth } from '../auth.jsx'
import { Link, useRouter } from '../router.jsx'
import { Alert, Empty, Progress, SessionBadge, Spinner, StaffBadge, Stat } from '../components/ui.jsx'
import { ROOM_TYPE_LABEL, dayKey, dayLabel, dayMonth, hour, minutes, shiftDay } from '../format.js'
import { ReviewFilters, defaultReviewFilter, reviewQuery } from '../components/ReviewFilters.jsx'

const TABS = [
	{ key: 'board', label: 'Ca dọn', icon: '📋', to: '/' },
	{ key: 'review', label: 'Duyệt ảnh', icon: '✅', to: '/review' },
	{ key: 'report', label: 'Báo cáo', icon: '📊', to: '/report' },
	{ key: 'reviews', label: 'Đánh giá khách', icon: '⭐', to: '/reviews' },
	{ key: 'staff', label: 'Cô dọn dẹp', icon: '👥', to: '/staff' },
	{ key: 'rooms', label: 'Phòng', icon: '🏠', to: '/rooms' },
	{ key: 'checklist', label: 'Mẫu checklist', icon: '📝', to: '/checklists' },
]

export function AdminShell({ active, children }) {
	const { user, logout } = useAuth()
	const { navigate } = useRouter()
	const [navOpen, setNavOpen] = useState(false)

	return (
		<div className="shell">
			<aside className={`side${navOpen ? ' side--open' : ''}`}>
				<div className="side-brand">
					<span className="side-mark">🧹</span>
					<div>
						<strong>Dọn dẹp</strong>
						<small>Checklist &amp; chấm công</small>
					</div>
				</div>
				<nav>
					{TABS.map((t) => (
						<Link key={t.key} to={t.to} className={`side-link${active === t.key ? ' active' : ''}`}
							onClick={() => setNavOpen(false)}>
							<span>{t.icon}</span>
							<span>{t.label}</span>
						</Link>
					))}
				</nav>
				<div className="side-foot">
					<div className="side-user">{user?.name}</div>
					<button
						className="btn btn--ghost"
						onClick={async () => {
							await logout()
							navigate('/login', { replace: true })
						}}
					>
						Đăng xuất
					</button>
				</div>
			</aside>

			<div className="main">
				<button className="nav-toggle" onClick={() => setNavOpen((v) => !v)}>
					☰ Menu
				</button>
				{children}
			</div>
		</div>
	)
}

// ─── Điều phối ca ─────────────────────────────────────────────────────────

export function BoardPage() {
	const { navigate } = useRouter()
	const today = dayKey()
	const [day, setDay] = useState(today)
	const [sessions, setSessions] = useState(null)
	const [staffs, setStaffs] = useState([])
	const [rooms, setRooms] = useState([])
	const [err, setErr] = useState('')
	const [syncing, setSyncing] = useState(false)
	const [msg, setMsg] = useState('')

	async function load(d) {
		setSessions(null)
		try {
			const [s, st, rm] = await Promise.all([api.sessions({ day: d }), api.staffs(), api.rooms()])
			setSessions(s.sessions || [])
			setStaffs((st.staffs || []).filter((x) => x.status === 'active'))
			setRooms(rm.rooms || [])
			setErr('')
		} catch (e) {
			setErr(e.message)
			setSessions([])
		}
	}

	useEffect(() => {
		load(day)
	}, [day])

	const rows = sessions || []
	const unassigned = rows.filter((s) => !s.staff_id).length
	const late = rows.filter((s) => s.late).length
	const waiting = rows.filter((s) => s.status === 'submitted').length
	const doing = rows.filter((s) => s.status === 'in_progress').length

	async function syncSchedule() {
		setSyncing(true)
		setErr('')
		setMsg('')
		try {
			const d = await api.syncSessions(14)
			setMsg(
				`Đồng bộ xong: thêm ${d.created} ca mới, ${d.skipped} ca đã có sẵn.` +
					(d.assigned ? ` Đã tự xếp người cho ${d.assigned} ca theo khu vực.` : ''),
			)
			await load(day)
		} catch (e) {
			setErr(e.message)
		} finally {
			setSyncing(false)
		}
	}

	async function assign(id, staffId) {
		try {
			const d = await api.assignSession(id, staffId)
			setSessions((list) => list.map((s) => (s.id === id ? d.session : s)))
		} catch (e) {
			setErr(e.message)
		}
	}

	return (
		<AdminShell active="board">
			<div className="page-head">
				<div>
					<h1>Ca dọn {dayLabel(day, today).toLowerCase()}</h1>
					<p className="sub">
						Ca sinh tự động từ lịch đặt phòng của Dayladau — không cần tạo tay. Mỗi lượt khách trả phòng là một ca
						dọn kỹ; phòng cho thuê theo giờ có thể nhiều ca trong ngày.
						<br />
						<strong>Xếp người:</strong> chọn tên ở cột “Cô phụ trách” ngay trong bảng, lưu ngay khi chọn.
					</p>
				</div>
				<div className="row-actions">
					<button className="btn btn--ghost" onClick={() => setDay(shiftDay(day, -1))}>‹</button>
					<button className={`btn${day === today ? ' btn--primary' : ' btn--ghost'}`} onClick={() => setDay(today)}>
						{dayLabel(day, today)}
					</button>
					<button className="btn btn--ghost" onClick={() => setDay(shiftDay(day, 1))}>›</button>
					<button className="btn btn--primary" disabled={syncing} onClick={syncSchedule}>
						{syncing ? 'Đang đồng bộ…' : 'Đồng bộ lịch'}
					</button>
				</div>
			</div>

			<div className="stats">
				<Stat label="Chưa có người nhận" value={unassigned} tone={unassigned ? 'danger' : 'ok'}
					sub={unassigned ? 'Chọn tên ở cột “Cô phụ trách”' : 'Đã xếp hết'} />
				<Stat label="Quá giờ khách vào" value={late} tone={late ? 'danger' : 'ok'}
					sub={late ? 'Gọi nhắc cô' : 'Đúng tiến độ'} />
				<Stat label="Đang dọn" value={doing} />
				<Stat label="Chờ đối soát" value={waiting} tone={waiting ? 'warn' : undefined} sub="Đã đủ ảnh" />
				<Stat label="Tổng ca" value={rows.length} sub={`${rooms.length} phòng đang quản lý`} />
			</div>

			<Alert>{err}</Alert>
			{msg && <Alert tone="ok">{msg}</Alert>}

			{sessions === null ? (
				<Spinner />
			) : !rows.length ? (
				<Empty icon="📅">Ngày này chưa có ca dọn nào. Bấm “Đồng bộ lịch” để kéo từ Dayladau.</Empty>
			) : (
				<div className="card table-wrap">
					<table>
						<thead>
							<tr>
								<th>Phòng</th>
								<th>Khung giờ</th>
								<th>Cô phụ trách</th>
								<th>Tiến độ ảnh</th>
								<th>Trạng thái</th>
								<th />
							</tr>
						</thead>
						<tbody>
							{rows.map((s) => (
								<tr key={s.id} className={s.late ? 'row--late' : ''}>
									<td>
										<div className="strong">{s.room?.name || s.room_id}</div>
										<div className="meta">
											{s.room?.code} · {ROOM_TYPE_LABEL[s.room?.room_type]} · chủ nhà {s.room?.host_name || '—'}
										</div>
										{s.guest_note && <div className="meta meta--warn">⚠ {s.guest_note}</div>}
									</td>
									<td className="nowrap">
										<div>
											<strong>{hour(s.checkout_at)}</strong> → {hour(s.deadline_at)}
										</div>
										<div className="meta">
											{s.next_checkin_at ? 'Có khách nhận phòng sau' : 'Không có khách kế tiếp'}
										</div>
									</td>
									<td>
										<select
											className={s.staff_id ? '' : 'select--todo'}
											value={s.staff_id || ''}
											onChange={(e) => assign(s.id, e.target.value)}
										>
											<option value="">— Chưa xếp —</option>
											{staffs.map((x) => (
												<option key={x.id} value={x.id}>
													{x.name}
												</option>
											))}
										</select>
									</td>
									<td className="nowrap">
										<Progress percent={s.progress?.percent} />
										<div className="meta">
											{s.progress?.done_required}/{s.progress?.total_required} mục · {s.progress?.photo_count} ảnh
										</div>
									</td>
									<td>
										<SessionBadge status={s.status} />
										{s.late && <div className="meta meta--danger">Quá giờ khách vào</div>}
									</td>
									<td className="nowrap">
										<button className="btn btn--ghost" onClick={() => navigate(`/sessions/${s.id}`)}>
											Xem ảnh
										</button>
									</td>
								</tr>
							))}
						</tbody>
					</table>
				</div>
			)}

		</AdminShell>
	)
}

// ─── Duyệt ảnh ────────────────────────────────────────────────────────

export function ReviewPage() {
	const { navigate } = useRouter()
	const [sessions, setSessions] = useState(null)
	const [err, setErr] = useState('')

	async function load() {
		setSessions(null)
		try {
			const d = await api.sessions({ status: 'submitted', from: Date.now() - 30 * 86400000 })
			// Cũ trước: ca chờ lâu nhất là ca cô sốt ruột nhất về tiền.
			setSessions((d.sessions || []).sort((a, b) => (a.submitted_at || 0) - (b.submitted_at || 0)))
			setErr('')
		} catch (e) {
			setErr(e.message)
			setSessions([])
		}
	}

	useEffect(() => {
		load()
	}, [])

	const rows = sessions || []
	const late = rows.filter((s) => s.deadline_at && s.submitted_at > s.deadline_at).length
	const photos = rows.reduce((n, x) => n + (x.progress?.photo_count || 0), 0)

	async function approve(id) {
		try {
			await api.reviewSession({ id, status: 'approved' })
			setSessions((list) => list.filter((s) => s.id !== id))
		} catch (e) {
			setErr(e.message)
		}
	}

	return (
		<AdminShell active="review">
			<div className="page-head">
				<div>
					<h1>Duyệt ảnh</h1>
					<p className="sub">Ca đã đủ ảnh, chờ bạn xem qua rồi duyệt — hoặc mở ra xem kỹ và trả lại kèm lý do.</p>
				</div>
				<button className="btn btn--ghost" onClick={load}>Tải lại</button>
			</div>

			<div className="stats">
				<Stat label="Ca chờ duyệt" value={rows.length} tone={rows.length ? 'warn' : 'ok'} />
				<Stat label="Ảnh cần xem" value={photos} />
				<Stat label="Xong sau giờ khách vào" value={late} tone={late ? 'warn' : undefined} />
			</div>

			<Alert>{err}</Alert>

			{sessions === null ? (
				<Spinner />
			) : !rows.length ? (
				<Empty icon="✅">Hết việc — không còn ca nào chờ duyệt.</Empty>
			) : (
				<div className="card table-wrap">
					<table>
						<thead>
							<tr>
								<th>Ca</th>
								<th>Cô dọn dẹp</th>
								<th>Nộp lúc</th>
								<th>Ảnh</th>
								<th className="right">Thời gian dọn</th>
								<th />
							</tr>
						</thead>
						<tbody>
							{rows.map((s) => {
								const over = s.deadline_at && s.submitted_at > s.deadline_at
								return (
									<tr key={s.id}>
										<td>
											<div className="strong">{s.room?.name || s.room_id}</div>
											<div className="meta">
												{s.room?.code} · {dayMonth(s.checkout_at)}
											</div>
										</td>
										<td>{s.staff_name || '—'}</td>
										<td className="nowrap">
											{dayMonth(s.submitted_at)} {hour(s.submitted_at)}
										</td>
										<td className="nowrap">
											{s.progress?.photo_count} ảnh / {s.progress?.total_required} mục
										</td>
										<td className="right nowrap">
											<strong>{minutes(s.minutes)}</strong>
											{over && <div className="meta meta--warn">xong sau hạn</div>}
										</td>
										<td className="nowrap">
											<button className="btn btn--ghost" onClick={() => navigate(`/sessions/${s.id}`)}>
												Xem ảnh
											</button>
											<button className="btn btn--primary" onClick={() => approve(s.id)}>
												Duyệt
											</button>
										</td>
									</tr>
								)
							})}
						</tbody>
					</table>
				</div>
			)}
		</AdminShell>
	)
}

// ─── Chi tiết ca ──────────────────────────────────────────────────────────

export function SessionDetailPage({ sessionId }) {
	const { navigate } = useRouter()
	const [session, setSession] = useState(null)
	const [err, setErr] = useState('')
	const [note, setNote] = useState('')
	const [preview, setPreview] = useState('')

	useEffect(() => {
		api
			.session(sessionId)
			.then((d) => {
				setSession(d.session)
				setNote(d.session.review_note || '')
			})
			.catch((e) => setErr(e.message))
	}, [sessionId])

	async function review(status) {
		try {
			const d = await api.reviewSession({ id: sessionId, status, note })
			setSession(d.session)
			setErr('')
		} catch (e) {
			setErr(e.message)
		}
	}

	if (!session) {
		return (
			<AdminShell active="review">
				{err ? <Alert>{err}</Alert> : <Spinner />}
			</AdminShell>
		)
	}

	const tpl = session.template_snapshot
	const p = session.progress || {}
	const reviewed = session.status === 'approved' || session.status === 'rejected'

	return (
		<AdminShell active="review">
			<button className="back" onClick={() => navigate('/review')}>‹ Về hàng chờ đối soát</button>

			<div className="page-head">
				<div>
					<h1>{session.room?.name || session.room_id}</h1>
					<p className="sub">
						{session.room?.code} · {ROOM_TYPE_LABEL[session.room?.room_type]} · {session.room?.address}
					</p>
				</div>
				<SessionBadge status={session.status} />
			</div>

			<Alert>{err}</Alert>

			<div className="grid-2">
				<div className="card pad">
					<KV k="Cô dọn dẹp" v={session.staff_name || 'Chưa xếp người'} />
					<KV k="Khách trả phòng" v={hour(session.checkout_at)} />
					<KV k="Hạn xong" v={`${hour(session.deadline_at)}${session.next_checkin_at ? ' (khách sau nhận phòng)' : ''}`} />
					<KV k="Bắt đầu → nộp" v={`${hour(session.started_at)} → ${hour(session.submitted_at)}`} />
					{session.guest_note && <KV k="Ghi chú từ khách" v={session.guest_note} />}
				</div>
				<div className="card pad">
					<KV k="Ảnh bắt buộc" v={`${p.done_required}/${p.total_required} mục · ${p.photo_count} ảnh`} />
					<KV k="Thời gian dọn" v={minutes(session.minutes)} total />
					{session.deadline_at > 0 && session.submitted_at > session.deadline_at && (
						<KV k="Xong sau hạn" v={`muộn ${minutes(Math.round((session.submitted_at - session.deadline_at) / 60000))}`} danger />
					)}
					{!!session.reviewed_at && <KV k="Đã xử lý" v={`${hour(session.reviewed_at)} bởi ${session.reviewed_by}`} />}
				</div>
			</div>

			{(tpl?.groups || []).map((g) => (
				<div key={g.id} className="card pad">
					<h2>{g.title}</h2>
					<div className="checks">
						{(g.items || []).map((it) => {
							const photos = session.items_state?.[it.id]?.photos || []
							const checked = !!session.items_state?.[it.id]?.checked
							const ok = it.require_photo ? photos.length >= (it.min_photos || 1) : checked
							return (
								<div key={it.id} className={`check${ok ? '' : ' check--missing'}`}>
									<div className="check-head">
										<span>{ok ? '✅' : '⬜'}</span>
										<span>{it.title}</span>
										{!it.require_photo && <span className="tag">không cần ảnh</span>}
										{it.require_photo && !ok && <span className="tag tag--danger">thiếu ảnh</span>}
									</div>
									{!!photos.length && (
										<div className="strip">
											{photos.map((ph, i) => (
												<img key={i} src={photoSrc(ph.url)} alt="" onClick={() => setPreview(photoSrc(ph.url))} />
											))}
										</div>
									)}
								</div>
							)
						})}
					</div>
				</div>
			))}

			<div className="sticky">
				<div className="sticky-note">
					{reviewed
						? `Ca đã ${session.status === 'approved' ? 'duyệt' : 'từ chối'}${session.review_note ? ` — ${session.review_note}` : ''}. Bấm lại để đổi quyết định.`
						: p.complete
							? 'Đủ ảnh bắt buộc. Xem qua rồi duyệt.'
							: `Còn thiếu ${(p.missing || []).length} mục ảnh — duyệt lúc này là xác nhận việc chưa có bằng chứng.`}
				</div>
				<div className="sticky-actions">
					<input placeholder="Ghi chú cho cô (cô sẽ đọc được)" value={note} onChange={(e) => setNote(e.target.value)} />
					<button className="btn btn--danger" onClick={() => review('rejected')}>Trả lại</button>
					<button className="btn btn--primary" onClick={() => review('approved')}>Duyệt</button>
				</div>
			</div>

			{preview && (
				<div className="lightbox" onClick={() => setPreview('')}>
					<img src={preview} alt="" />
					<button className="lightbox-x">✕</button>
				</div>
			)}
		</AdminShell>
	)
}

function KV({ k, v, total, danger }) {
	return (
		<div className={`kv${total ? ' kv--total' : ''}`}>
			<span>{k}</span>
			<strong className={danger ? 'danger' : ''}>{v}</strong>
		</div>
	)
}

// ─── Báo cáo hiệu suất ────────────────────────────────────────────────────
//
// Phần mềm này không tính lương. Báo cáo trả lời: hôm nay chạy được bao nhiêu ca,
// bao nhiêu phòng, dọn trung bình bao lâu, có ca nào xong sau giờ khách vào không.

export function ReportPage() {
	const [day, setDay] = useState(dayKey())
	const [data, setData] = useState(null)
	const [err, setErr] = useState('')
	const [open, setOpen] = useState('')

	useEffect(() => {
		setData(null)
		api
			.report({ day })
			.then(setData)
			.catch((e) => {
				setErr(e.message)
				setData({ rows: [], sessions: [], total: {} })
			})
	}, [day])

	const rows = data?.rows || []
	const total = data?.total || {}
	const today = dayKey()
	const sessionsByStaff = useMemo(() => {
		const m = {}
		;(data?.sessions || []).forEach((s) => {
			if (!s.staff_id) return
			;(m[s.staff_id] = m[s.staff_id] || []).push(s)
		})
		return m
	}, [data])

	return (
		<AdminShell active="report">
			<div className="page-head">
				<div>
					<h1>Báo cáo {dayLabel(day, today).toLowerCase()}</h1>
					<p className="sub">
						Số ca dọn, số phòng và thời gian trung bình. Phần mềm không tính lương — lương theo cơ chế riêng.
					</p>
				</div>
				<div className="row-actions">
					<button className="btn btn--ghost" onClick={() => setDay(shiftDay(day, -1))}>‹</button>
					<button className={`btn${day === today ? ' btn--primary' : ' btn--ghost'}`} onClick={() => setDay(today)}>
						{dayLabel(day, today)}
					</button>
					<button className="btn btn--ghost" disabled={day >= today} onClick={() => setDay(shiftDay(day, 1))}>›</button>
				</div>
			</div>

			<div className="stats">
				<Stat label="Ca đã dọn xong" value={total.sessions || 0} sub={`${rows.length} cô làm việc`} />
				<Stat label="Số phòng" value={total.rooms || 0} />
				<Stat label="Thời gian TB mỗi ca" value={minutes(total.avg_minute)} />
				<Stat label="Chờ duyệt ảnh" value={total.pending || 0} tone={total.pending ? 'warn' : undefined} />
				<Stat label="Xong sau giờ khách vào" value={total.late || 0} tone={total.late ? 'danger' : 'ok'} />
			</div>

			<Alert>{err}</Alert>

			{data === null ? (
				<Spinner />
			) : !rows.length ? (
				<Empty icon="📊">Ngày này chưa có ca nào hoàn tất.</Empty>
			) : (
				<div className="card table-wrap">
					<table>
						<thead>
							<tr>
								<th>Cô dọn dẹp</th>
								<th className="right">Ca</th>
								<th className="right">Phòng</th>
								<th className="right">TB mỗi ca</th>
								<th className="right">Ảnh</th>
								<th className="right">Đã duyệt</th>
								<th className="right">Chờ duyệt</th>
								<th className="right">Trả lại</th>
								<th className="right">Trễ</th>
							</tr>
						</thead>
						<tbody>
							{rows.map((r) => (
								<Fragment key={r.staff_id}>
									<tr className="clickable" onClick={() => setOpen(open === r.staff_id ? '' : r.staff_id)}>
										<td>
											<div className="strong">{open === r.staff_id ? '⌄' : '›'} {r.name}</div>
											<div className="meta">{r.phone}</div>
										</td>
										<td className="right"><strong>{r.sessions}</strong></td>
										<td className="right">{r.rooms}</td>
										<td className="right">{minutes(r.avg_minute)}</td>
										<td className="right">{r.photos}</td>
										<td className="right">{r.approved}</td>
										<td className="right warn">{r.pending || '—'}</td>
										<td className="right">{r.rejected || '—'}</td>
										<td className="right">{r.late ? <span className="danger">{r.late}</span> : '—'}</td>
									</tr>
									{open === r.staff_id && (
										<tr>
											<td colSpan="9" className="subcell">
												<table className="subtable">
													<tbody>
														{(sessionsByStaff[r.staff_id] || [])
															.sort((a, b) => a.checkout_at - b.checkout_at)
															.map((s) => (
																<tr key={s.id}>
																	<td className="nowrap">{hour(s.checkout_at)}</td>
																	<td>{s.room?.name || s.room_id}</td>
																	<td className="right">{minutes(s.minutes)}</td>
																	<td><SessionBadge status={s.status} /></td>
																	<td><Link to={`/sessions/${s.id}`}>xem ảnh</Link></td>
																</tr>
															))}
													</tbody>
												</table>
											</td>
										</tr>
									)}
								</Fragment>
							))}
						</tbody>
					</table>
				</div>
			)}
		</AdminShell>
	)
}

// ─── Đánh giá của khách ───────────────────────────────────────────────────
//
// Lấy từ API công khai của Dayladau. Điểm `cleanliness` là thước đo trực tiếp
// nhất chất lượng dọn dẹp; phần "Cần xử lý" lọc riêng review chê chuyện sạch sẽ
// để quản lý không phải đọc hết vài trăm đánh giá chung chung.

export function ReviewsPage() {
	const [filter, setFilter] = useState(defaultReviewFilter)
	const [data, setData] = useState(null)
	const [err, setErr] = useState('')
	const [syncing, setSyncing] = useState(false)
	const [msg, setMsg] = useState('')

	async function load(f) {
		setData(null)
		try {
			setData(await api.reviews(reviewQuery(f)))
			setErr('')
		} catch (e) {
			setErr(e.message)
			setData({ stats: null, reviews: [] })
		}
	}

	useEffect(() => {
		load(filter)
	}, [filter])

	async function sync() {
		setSyncing(true)
		setErr('')
		setMsg('')
		try {
			const d = await api.syncReviews(180)
			setMsg(`Đã tải về ${d.synced} đánh giá từ Dayladau.`)
			await load(filter)
		} catch (e) {
			setErr(e.message)
		} finally {
			setSyncing(false)
		}
	}

	const st = data?.stats
	const list = data?.reviews || []

	return (
		<AdminShell active="reviews">
			<div className="page-head">
				<div>
					<h1>Đánh giá của khách</h1>
					<p className="sub">
						Lấy từ Dayladau. Các cô cũng xem và lọc được phần này trên điện thoại.
						{data?.last_sync_at ? ` Cập nhật lần cuối ${dayMonth(data.last_sync_at)} ${hour(data.last_sync_at)}.` : ''}
					</p>
				</div>
				<button className="btn btn--primary" disabled={syncing} onClick={sync}>
					{syncing ? 'Đang tải…' : 'Tải đánh giá mới'}
				</button>
			</div>

			<ReviewFilters
				value={filter}
				onChange={setFilter}
				rooms={data?.rooms || []}
				facilities={data?.facilities || []}
				starCounts={data?.star_counts || {}}
			/>

			<Alert>{err}</Alert>
			{msg && <Alert tone="ok">{msg}</Alert>}

			{data === null ? (
				<Spinner />
			) : !list.length ? (
				<Empty icon="⭐">
					Không có đánh giá nào khớp bộ lọc. Chưa tải bao giờ thì bấm “Tải đánh giá mới”.
				</Empty>
			) : (
				<>
					<div className="stats">
						<Stat label="Đánh giá khớp lọc" value={st.total} />
						<Stat label="Điểm sạch sẽ TB" value={st.avg_cleanliness ? st.avg_cleanliness.toFixed(2) : '—'}
							tone={st.avg_cleanliness && st.avg_cleanliness < 4.5 ? 'warn' : 'ok'} />
						<Stat label="Điểm chung TB" value={st.avg_overall ? st.avg_overall.toFixed(2) : '—'} />
						<Stat label="Chê chưa sạch" value={st.low_clean} tone={st.low_clean ? 'danger' : 'ok'}
							sub="điểm sạch sẽ ≤ 3" />
						<Stat label="Có nhắc dọn dẹp" value={st.about_cleaning} />
					</div>

					{st.need_attention?.length > 0 && (
						<div className="card pad">
							<h2>Cần xử lý</h2>
							{st.need_attention.map((r) => (
								<ReviewRow key={r.id} r={r} />
							))}
						</div>
					)}

					<div className="card pad">
						<h2>Tất cả ({list.length})</h2>
						{list.map((r) => (
							<ReviewRow key={r.id} r={r} />
						))}
					</div>
				</>
			)}
		</AdminShell>
	)
}

function ReviewRow({ r }) {
	return (
		<div className={`rv${r.overall >= 4 ? ' rv--good' : ''}${r.cleanliness > 0 && r.cleanliness <= 3 ? ' rv--bad' : ''}`}>
			<div className="rv-top">
				<span>{'⭐'.repeat(Math.max(1, r.overall))}</span>
				<span className="rv-room">
					{r.room_code} · {r.listing_name}
				</span>
				{r.facility_label && <span className="tag">{r.facility_label}</span>}
				{r.cleanliness > 0 && <span className="tag">sạch sẽ {r.cleanliness}/5</span>}
				{r.about_cleaning && <span className="tag tag--danger">nhắc dọn dẹp</span>}
				<span className="rv-date">{dayMonth(r.created_at)}</span>
			</div>
			{r.comment && <div className="rv-text">{r.comment}</div>}
		</div>
	)
}

// ─── Nhân sự ──────────────────────────────────────────────────────────────

export function StaffPage() {
	const [staffs, setStaffs] = useState(null)
	const [err, setErr] = useState('')

	async function load() {
		setStaffs(null)
		try {
			const d = await api.staffs()
			setStaffs(d.staffs || [])
			setErr('')
		} catch (e) {
			setErr(e.message)
			setStaffs([])
		}
	}

	useEffect(() => {
		load()
	}, [])

	async function setStatus(id, status) {
		try {
			const d = await api.reviewStaff(id, status)
			setStaffs((list) => list.map((s) => (s.id === id ? d.staff : s)))
		} catch (e) {
			setErr(e.message)
		}
	}

	// Hồ sơ chờ duyệt lên đầu: cô đăng ký xong mà không ai bấm duyệt thì hôm sau
	// không đăng nhập được và sẽ gọi điện.
	const order = { pending: 0, active: 1, suspended: 2, rejected: 3 }
	const rows = [...(staffs || [])].sort((a, b) => (order[a.status] ?? 9) - (order[b.status] ?? 9))
	const pending = rows.filter((s) => s.status === 'pending').length

	return (
		<AdminShell active="staff">
			<div className="page-head">
				<div>
					<h1>Cô dọn dẹp</h1>
					<p className="sub">Cô tự đăng ký bằng số điện thoại rồi chờ bạn duyệt ở đây mới đăng nhập được.</p>
				</div>
				<button className="btn btn--ghost" onClick={load}>Tải lại</button>
			</div>

			<div className="stats">
				<Stat label="Chờ duyệt" value={pending} tone={pending ? 'warn' : 'ok'}
					sub={pending ? 'Cô chưa đăng nhập được' : 'Không tồn đọng'} />
				<Stat label="Đang làm" value={rows.filter((s) => s.status === 'active').length} />
				<Stat label="Tổng hồ sơ" value={rows.length} />
			</div>

			<Alert>{err}</Alert>

			{staffs === null ? (
				<Spinner />
			) : (
				<div className="card table-wrap">
					<table>
						<thead>
							<tr>
								<th>Họ tên</th>
								<th>Số điện thoại</th>
								<th>Khu vực</th>
								<th>Ghi chú</th>
								<th>Trạng thái</th>
								<th />
							</tr>
						</thead>
						<tbody>
							{rows.map((s) => (
								<tr key={s.id} className={s.status === 'pending' ? 'row--attention' : ''}>
									<td className="strong">{s.name}</td>
									<td className="nowrap">{s.phone}</td>
									<td>{(s.zones || []).join(', ') || '—'}</td>
									<td>{s.note || '—'}</td>
									<td><StaffBadge status={s.status} /></td>
									<td className="nowrap">
										{s.status === 'pending' ? (
											<>
												<button className="btn btn--primary" onClick={() => setStatus(s.id, 'active')}>Duyệt</button>
												<button className="btn btn--danger" onClick={() => setStatus(s.id, 'rejected')}>Từ chối</button>
											</>
										) : s.status === 'active' ? (
											<button className="btn btn--ghost" onClick={() => setStatus(s.id, 'suspended')}>Tạm khoá</button>
										) : (
											<button className="btn btn--ghost" onClick={() => setStatus(s.id, 'active')}>Mở lại</button>
										)}
									</td>
								</tr>
							))}
						</tbody>
					</table>
				</div>
			)}
		</AdminShell>
	)
}

// ─── Phòng ────────────────────────────────────────────────────────────────

export function RoomsPage() {
	const [rooms, setRooms] = useState(null)
	const [templates, setTemplates] = useState([])
	const [err, setErr] = useState('')
	const [msg, setMsg] = useState('')
	const [syncing, setSyncing] = useState(false)
	const [edit, setEdit] = useState(null)

	async function load() {
		try {
			const [r, t] = await Promise.all([api.rooms(), api.templates()])
			setRooms(r.rooms || [])
			setTemplates(t.templates || [])
		} catch (e) {
			setErr(e.message)
			setRooms([])
		}
	}

	useEffect(() => {
		load()
	}, [])

	async function sync() {
		setSyncing(true)
		setErr('')
		setMsg('')
		try {
			const d = await api.syncRooms(60)
			setMsg(`Đồng bộ xong: thêm ${d.added} phòng, cập nhật ${d.updated} phòng.`)
			await load()
		} catch (e) {
			setErr(e.message)
		} finally {
			setSyncing(false)
		}
	}

	const rows = rooms || []

	return (
		<AdminShell active="rooms">
			<div className="page-head">
				<div>
					<h1>Phòng</h1>
					<p className="sub">
						Danh sách lấy thật từ api.dayladau.com. Mẫu checklist và hướng dẫn vào nhà do bạn sửa — đồng bộ lại không
						ghi đè. Đệm dọn dẹp lấy theo cài đặt của listing bên Dayladau.
					</p>
				</div>
				<button className="btn btn--primary" onClick={sync} disabled={syncing}>
					{syncing ? 'Đang đồng bộ…' : 'Đồng bộ từ Dayladau'}
				</button>
			</div>

			<Alert>{err}</Alert>
			{msg && <Alert tone="ok">{msg}</Alert>}

			{rooms === null ? (
				<Spinner />
			) : !rows.length ? (
				<Empty icon="🏠">Chưa có phòng nào. Bấm “Đồng bộ từ Dayladau” để kéo về.</Empty>
			) : (
				<div className="card table-wrap">
					<table>
						<thead>
							<tr>
								<th>Phòng</th>
								<th>Khu vực</th>
								<th>Loại</th>
								<th className="right">Đệm dọn dẹp</th>
								<th>Mẫu checklist</th>
								<th />
							</tr>
						</thead>
						<tbody>
							{rows.map((r) => (
								<tr key={r.id}>
									<td>
										<div className="strong">{r.name}</div>
										<div className="meta">{r.code} · {r.address}</div>
										<div className="meta">Chủ nhà: {r.host_name || '—'}</div>
									</td>
									<td>{r.zone}</td>
									<td>{ROOM_TYPE_LABEL[r.room_type] || r.room_type}</td>
									<td className="right">{r.clean_time || 1}h</td>
									<td>{templates.find((t) => t.id === r.template_id)?.name || '—'}</td>
									<td><button className="btn btn--ghost" onClick={() => setEdit(r)}>Sửa</button></td>
								</tr>
							))}
						</tbody>
					</table>
				</div>
			)}

			{edit && (
				<RoomModal
					room={edit}
					templates={templates}
					onClose={() => setEdit(null)}
					onSaved={() => {
						setEdit(null)
						load()
					}}
				/>
			)}
		</AdminShell>
	)
}

function RoomModal({ room, templates, onClose, onSaved }) {
	const [templateId, setTemplateId] = useState(room.template_id || '')
	const [doorNote, setDoorNote] = useState(room.door_note || '')
	const [err, setErr] = useState('')
	const [busy, setBusy] = useState(false)

	async function submit() {
		setBusy(true)
		setErr('')
		try {
			await api.saveRoom({ id: room.id, template_id: templateId, door_note: doorNote })
			onSaved()
		} catch (e) {
			setErr(e.message)
		} finally {
			setBusy(false)
		}
	}

	return (
		<div className="modal" onClick={onClose}>
			<div className="modal-in" onClick={(e) => e.stopPropagation()}>
				<h2>{room.name}</h2>
				<label className="field">
					<span>Mẫu checklist</span>
					<select value={templateId} onChange={(e) => setTemplateId(e.target.value)}>
						<option value="">— Chưa gán —</option>
						{templates.map((t) => (
							<option key={t.id} value={t.id}>{t.name}</option>
						))}
					</select>
				</label>
				<label className="field">
					<span>Hướng dẫn vào nhà (cô đọc trên điện thoại)</span>
					<input value={doorNote} onChange={(e) => setDoorNote(e.target.value)} placeholder="VD: khoá số 2580, tầng 4" />
				</label>
				<Alert>{err}</Alert>
				<div className="modal-actions">
					<button className="btn btn--ghost" onClick={onClose}>Huỷ</button>
					<button className="btn btn--primary" onClick={submit} disabled={busy}>
						{busy ? 'Đang lưu…' : 'Lưu'}
					</button>
				</div>
			</div>
		</div>
	)
}

// ─── Mẫu checklist ────────────────────────────────────────────────────────

let uid = 0
const newId = (p) => `${p}_new${++uid}`

export function ChecklistPage() {
	const [templates, setTemplates] = useState(null)
	const [activeId, setActiveId] = useState('')
	const [draft, setDraft] = useState(null)
	const [err, setErr] = useState('')
	const [msg, setMsg] = useState('')
	const [busy, setBusy] = useState(false)

	useEffect(() => {
		api
			.templates()
			.then((d) => {
				const list = d.templates || []
				setTemplates(list)
				if (list.length) {
					setActiveId(list[0].id)
					setDraft(structuredClone(list[0]))
				}
			})
			.catch((e) => setErr(e.message))
	}, [])

	function pick(id) {
		setActiveId(id)
		const t = (templates || []).find((x) => x.id === id)
		if (t) setDraft(structuredClone(t))
		setMsg('')
	}

	if (templates === null) {
		return <AdminShell active="checklist"><Spinner /></AdminShell>
	}
	if (!draft) {
		return <AdminShell active="checklist"><Empty icon="📝">Chưa có mẫu checklist nào.</Empty></AdminShell>
	}

	const allItems = (draft.groups || []).flatMap((g) => g.items || [])
	const requiredCount = allItems.filter((i) => i.require_photo).length
	const photoCount = allItems.reduce((s, i) => s + (i.require_photo ? i.min_photos || 1 : 0), 0)

	function patchItem(gid, iid, patch) {
		setDraft((d) => ({
			...d,
			groups: d.groups.map((g) =>
				g.id !== gid ? g : { ...g, items: g.items.map((it) => (it.id === iid ? { ...it, ...patch } : it)) },
			),
		}))
	}

	async function save() {
		setBusy(true)
		setErr('')
		setMsg('')
		try {
			const d = await api.saveTemplate(draft)
			setTemplates((list) => list.map((t) => (t.id === d.template.id ? d.template : t)))
			setMsg('Đã lưu. Ca tạo từ giờ dùng mẫu mới; ca đang dở vẫn giữ mẫu cũ để không mất ảnh.')
		} catch (e) {
			setErr(e.message)
		} finally {
			setBusy(false)
		}
	}

	return (
		<AdminShell active="checklist">
			<div className="page-head">
				<div>
					<h1>Mẫu checklist</h1>
					<p className="sub">Gán theo loại phòng. Ca đang dở vẫn dùng mẫu lúc tạo, nên sửa ở đây không làm mất ảnh của ai.</p>
				</div>
				<button className="btn btn--primary" onClick={save} disabled={busy}>
					{busy ? 'Đang lưu…' : 'Lưu mẫu'}
				</button>
			</div>

			<div className="tabs">
				{templates.map((t) => (
					<button key={t.id} className={`tab${t.id === activeId ? ' active' : ''}`} onClick={() => pick(t.id)}>
						{t.name}
					</button>
				))}
			</div>

			<div className="stats">
				<Stat label="Tổng số việc" value={allItems.length} />
				<Stat label="Việc bắt buộc chụp ảnh" value={requiredCount} sub="Thiếu là không nộp được ca" />
				<Stat label="Số ảnh tối thiểu mỗi ca" value={photoCount} tone={photoCount > 20 ? 'warn' : undefined}
					sub={photoCount > 20 ? 'Nhiều quá thì cô bỏ cuộc' : undefined} />
			</div>

			<Alert>{err}</Alert>
			{msg && <Alert tone="ok">{msg}</Alert>}

			<div className="card pad">
				<label className="field">
					<span>Tên mẫu</span>
					<input value={draft.name} onChange={(e) => setDraft((d) => ({ ...d, name: e.target.value }))} />
				</label>
			</div>

			{draft.groups.map((g) => (
				<div key={g.id} className="card pad">
					<div className="group-edit-head">
						<input
							className="title-input"
							value={g.title}
							onChange={(e) =>
								setDraft((d) => ({ ...d, groups: d.groups.map((x) => (x.id === g.id ? { ...x, title: e.target.value } : x)) }))
							}
						/>
						<button
							className="btn btn--danger"
							onClick={() => setDraft((d) => ({ ...d, groups: d.groups.filter((x) => x.id !== g.id) }))}
						>
							Xoá nhóm
						</button>
					</div>

					{g.items.map((it) => (
						<div key={it.id} className="item-edit">
							<input
								placeholder="Tên việc, VD: Thay ga gối mới"
								value={it.title}
								onChange={(e) => patchItem(g.id, it.id, { title: e.target.value })}
							/>
							<input
								placeholder="Gợi ý cho cô (không bắt buộc)"
								value={it.hint || ''}
								onChange={(e) => patchItem(g.id, it.id, { hint: e.target.value })}
							/>
							<label className="check-inline">
								<input
									type="checkbox"
									checked={!!it.require_photo}
									onChange={(e) => patchItem(g.id, it.id, { require_photo: e.target.checked })}
								/>
								<span>Bắt buộc ảnh</span>
							</label>
							<input
								className="w-num"
								type="number"
								min="1"
								max="6"
								title="Số ảnh tối thiểu"
								disabled={!it.require_photo}
								value={it.min_photos || 1}
								onChange={(e) => patchItem(g.id, it.id, { min_photos: Number(e.target.value) || 1 })}
							/>
							<button
								className="btn btn--ghost"
								onClick={() =>
									setDraft((d) => ({
										...d,
										groups: d.groups.map((x) => (x.id === g.id ? { ...x, items: x.items.filter((y) => y.id !== it.id) } : x)),
									}))
								}
							>
								✕
							</button>
						</div>
					))}

					<button
						className="btn btn--ghost"
						onClick={() =>
							setDraft((d) => ({
								...d,
								groups: d.groups.map((x) =>
									x.id === g.id
										? { ...x, items: [...x.items, { id: newId('i'), title: '', require_photo: true, min_photos: 1 }] }
										: x,
								),
							}))
						}
					>
						+ Thêm việc
					</button>
				</div>
			))}

			<button
				className="btn btn--ghost"
				onClick={() => setDraft((d) => ({ ...d, groups: [...d.groups, { id: newId('g'), title: 'Nhóm việc mới', items: [] }] }))}
			>
				+ Thêm nhóm việc
			</button>
		</AdminShell>
	)
}
