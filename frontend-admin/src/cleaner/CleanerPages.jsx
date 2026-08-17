// Ba màn của cô dọn dẹp: ca hôm nay, checklist một phòng, kết quả của tôi.
//
// Thiết kế cho điện thoại tầm trung, tay ướt, ánh đèn phòng tắm: chữ ≥15px,
// vùng chạm ≥44px, điều hướng ở đáy màn hình vừa tầm ngón cái.
//
// KHÔNG có tiền ở đây. Lương tính theo cơ chế riêng ngoài phần mềm; màn này chỉ
// cho cô thấy việc đã làm và khách đánh giá thế nào.

import { useEffect, useMemo, useState } from 'react'
import { api } from '../api.js'
import { useAuth } from '../auth.jsx'
import { useRouter } from '../router.jsx'
import PhotoUploader from '../components/PhotoUploader.jsx'
import { Alert, Empty, Progress, Spinner } from '../components/ui.jsx'
import CaptureFlow from './CaptureFlow.jsx'
import { ReviewFilters, defaultReviewFilter, reviewQuery } from '../components/ReviewFilters.jsx'
import {
	ROOM_TYPE_LABEL,
	SESSION_LABEL,
	dayKey,
	dayMonth,
	hour,
	minutes,
	shiftDay,
} from '../format.js'

export function CleanerShell({ active, title, back, children }) {
	const { logout, user } = useAuth()
	const { navigate } = useRouter()

	return (
		<div className="mob">
			<header className="mob-top">
				{back ? (
					<button className="mob-btn" onClick={() => navigate(back)} aria-label="Quay lại">‹</button>
				) : (
					<span className="mob-btn mob-btn--hidden" />
				)}
				<div className="mob-title">
					<span>{title}</span>
					{user && <small>{user.name}</small>}
				</div>
				<button
					className="mob-btn"
					aria-label="Đăng xuất"
					onClick={async () => {
						await logout()
						navigate('/login', { replace: true })
					}}
				>
					⎋
				</button>
			</header>

			<main className="mob-body">{children}</main>

			<nav className="mob-tabs">
				<button className={`mob-tab${active === 'today' ? ' active' : ''}`} onClick={() => navigate('/cleaning')}>
					<span>📋</span>
					<span>Ca hôm nay</span>
				</button>
				<button className={`mob-tab${active === 'me' ? ' active' : ''}`} onClick={() => navigate('/cleaning/me')}>
					<span>📊</span>
					<span>Kết quả</span>
				</button>
			</nav>
		</div>
	)
}

// ─── Ca hôm nay ───────────────────────────────────────────────────────────

export function CleanerTodayPage() {
	const { navigate } = useRouter()
	const [sessions, setSessions] = useState(null)
	const [err, setErr] = useState('')

	useEffect(() => {
		api
			.sessions({ day: dayKey() })
			.then((d) => setSessions(d.sessions || []))
			.catch((e) => {
				setErr(e.message)
				setSessions([])
			})
	}, [])

	// Sắp theo GIỜ PHẢI XONG, không theo tên phòng: cô mở app ra để biết đi đâu
	// trước, và phòng có khách nhận lúc 14h phải nằm trên phòng trống cả chiều.
	const rows = useMemo(
		() => [...(sessions || [])].sort((a, b) => (a.deadline_at || 0) - (b.deadline_at || 0)),
		[sessions],
	)

	const remaining = rows.filter((s) => s.status === 'todo' || s.status === 'in_progress').length
	const done = rows.filter((s) => s.status === 'submitted' || s.status === 'approved').length

	return (
		<CleanerShell active="today" title="Ca hôm nay">
			<div className="mob-sum">
				<div>
					<strong>{remaining}</strong>
					<span>phòng còn phải dọn</span>
				</div>
				<div>
					<strong>{done}</strong>
					<span>phòng đã xong</span>
				</div>
			</div>

			<Alert>{err}</Alert>

			{sessions === null ? (
				<Spinner />
			) : !rows.length ? (
				<Empty icon="☕">Hôm nay bạn chưa được xếp phòng nào. Nghỉ ngơi nhé!</Empty>
			) : (
				<div className="mob-list">
					{rows.map((s) => {
						const finished = s.status === 'submitted' || s.status === 'approved'
						return (
							<button
								key={s.id}
								className={`mob-card${finished ? ' mob-card--done' : ''}`}
								onClick={() => navigate(`/cleaning/session/${s.id}`)}
							>
								<div className="mob-card-top">
									<span className="mob-card-time">
										🕐 Xong trước <strong>{hour(s.deadline_at)}</strong>
									</span>
									<span className={`pill pill--${s.status}`}>{SESSION_LABEL[s.status]}</span>
								</div>

								<div className="mob-card-name">{s.room?.name || s.room_id}</div>
								<div className="mob-card-addr">📍 {s.room?.address}</div>
								<div className="mob-card-meta">
									{ROOM_TYPE_LABEL[s.room?.room_type] || s.room?.room_type} · khách trả phòng {hour(s.checkout_at)}
									{s.next_checkin_at ? ' · có khách vào ngay sau' : ''}
								</div>

								{s.guest_note && <div className="mob-note">Lưu ý: {s.guest_note}</div>}
								{s.late && <div className="mob-note mob-note--danger">Đã quá giờ khách vào</div>}

								<div className="mob-card-foot">
									<Progress percent={s.progress?.percent} />
									<span>
										{s.progress?.done_required}/{s.progress?.total_required} việc
									</span>
									{finished && !!s.minutes && <strong>{minutes(s.minutes)}</strong>}
								</div>

								{s.status === 'rejected' && s.review_note && (
									<div className="mob-note mob-note--danger">Quản lý nhắn: {s.review_note}</div>
								)}
							</button>
						)
					})}
				</div>
			)}
		</CleanerShell>
	)
}

// ─── Checklist một phòng ──────────────────────────────────────────────────

export function CleanerSessionPage({ sessionId }) {
	const { navigate } = useRouter()
	const [session, setSession] = useState(null)
	const [err, setErr] = useState('')

	useEffect(() => {
		api
			.session(sessionId)
			.then((d) => setSession(d.session))
			.catch((e) => setErr(e.message))
	}, [sessionId])

	if (err) {
		return (
			<CleanerShell active="today" title="Không mở được" back="/cleaning">
				<Alert>{err}</Alert>
			</CleanerShell>
		)
	}
	if (!session) {
		return (
			<CleanerShell active="today" title="Đang tải…" back="/cleaning">
				<Spinner />
			</CleanerShell>
		)
	}

	// Ca đã chốt thì không chụp nữa — hiện lại ảnh cho cô xem.
	if (session.status === 'approved' || session.status === 'rejected') {
		return (
			<CleanerShell active="today" title={session.room?.name || 'Ca dọn'} back="/cleaning">
				<div className={`banner banner--${session.status === 'approved' ? 'ok' : 'danger'}`}>
					<strong>
						{session.status === 'approved' ? 'Quản lý đã duyệt ca này.' : 'Quản lý trả lại ca này.'}
					</strong>
					{session.review_note && <span>{session.review_note}</span>}
				</div>
				<div className="mob-head">
					<div>📍 {session.room?.address}</div>
					<div>
						{session.progress?.photo_count} ảnh · {minutes(session.minutes)}
					</div>
				</div>
			</CleanerShell>
		)
	}

	// Toàn màn hình, không header/tabbar: mỗi pixel dành cho việc đang làm.
	return (
		<CaptureFlow
			session={session}
			onSessionChange={setSession}
			onExit={() => navigate('/cleaning')}
		/>
	)
}

// ─── Kết quả của tôi ──────────────────────────────────────────────────────

export function CleanerMePage() {
	const [day, setDay] = useState(dayKey())
	const [data, setData] = useState(null)
	const [err, setErr] = useState('')

	// Bộ lọc đánh giá tách riêng khỏi ô chọn ngày của phần số liệu: cô xem hiệu
	// suất của MỘT ngày, nhưng đánh giá thì nhìn cả tháng mới thấy xu hướng.
	const [filter, setFilter] = useState(defaultReviewFilter)
	const [reviews, setReviews] = useState(null)
	const [showFilter, setShowFilter] = useState(false)

	useEffect(() => {
		setData(null)
		api
			.report({ day })
			.then(setData)
			.catch((e) => {
				setErr(e.message)
				setData({ rows: [], sessions: [] })
			})
	}, [day])

	useEffect(() => {
		setReviews(null)
		api
			.reviews(reviewQuery(filter))
			.then(setReviews)
			.catch(() => setReviews({ stats: null, reviews: [] }))
	}, [filter])

	const row = data?.rows?.[0]
	const today = dayKey()
	const st = reviews?.stats
	const list = reviews?.reviews || []
	const activeFilters =
		(filter.room_id ? 1 : 0) + (filter.facility_id ? 1 : 0) + (filter.stars || []).length

	return (
		<CleanerShell active="me" title="Kết quả của tôi">
			<div className="mob-month">
				<button onClick={() => setDay(shiftDay(day, -1))} aria-label="Hôm trước">‹</button>
				<span>{day === today ? 'Hôm nay' : dayMonth(new Date(day).getTime())}</span>
				<button disabled={day >= today} onClick={() => setDay(shiftDay(day, 1))} aria-label="Hôm sau">›</button>
			</div>

			{data === null ? (
				<Spinner />
			) : (
				<div className="mob-sum mob-sum--3">
					<div>
						<strong>{row?.sessions || 0}</strong>
						<span>ca đã dọn</span>
					</div>
					<div>
						<strong>{row?.rooms || 0}</strong>
						<span>phòng</span>
					</div>
					<div>
						<strong>{minutes(row?.avg_minute)}</strong>
						<span>trung bình mỗi ca</span>
					</div>
				</div>
			)}

			{!!row?.late && (
				<div className="banner banner--warn">
					<strong>{row.late} ca xong sau giờ khách vào</strong>
					<span>Nếu do phòng quá bẩn hoặc thiếu đồ, báo quản lý để xếp thêm thời gian.</span>
				</div>
			)}

			<Alert>{err}</Alert>

			{/* Review là thước đo trực tiếp nhất việc dọn dẹp — cho cô thấy để tự
			    cải thiện, không phải để quản lý giữ riêng làm cơ sở khiển trách. */}
			<div className="mob-h2-row">
				<h2 className="mob-h2">Khách đánh giá</h2>
				<button className="btn btn--ghost" onClick={() => setShowFilter((v) => !v)}>
					Lọc{activeFilters ? ` (${activeFilters})` : ''} {showFilter ? '⌃' : '⌄'}
				</button>
			</div>

			{/* Bộ lọc gập lại mặc định: màn điện thoại chật, mở sẵn thì đẩy hết đánh
			    giá xuống dưới màn và cô phải cuộn mới thấy thứ mình cần xem. */}
			{showFilter && (
				<ReviewFilters
					compact
					value={filter}
					onChange={setFilter}
					rooms={reviews?.rooms || []}
					facilities={reviews?.facilities || []}
					starCounts={reviews?.star_counts || {}}
				/>
			)}

			{reviews === null ? (
				<Spinner />
			) : !list.length ? (
				<Empty icon="⭐">Không có đánh giá nào khớp bộ lọc.</Empty>
			) : (
				<>
					<div className="mob-sum">
						<div>
							<strong>{st?.avg_cleanliness ? st.avg_cleanliness.toFixed(1) : '—'}</strong>
							<span>điểm sạch sẽ trung bình</span>
						</div>
						<div>
							<strong>{st?.low_clean || 0}</strong>
							<span>lượt chê chưa sạch</span>
						</div>
					</div>

					<div className="mob-list">
						{list.map((r) => (
							<div
								key={r.id}
								className={`rv${r.overall >= 4 ? ' rv--good' : ''}${r.cleanliness > 0 && r.cleanliness <= 3 ? ' rv--bad' : ''}`}
							>
								<div className="rv-top">
									<span>{'⭐'.repeat(Math.max(1, r.overall))}</span>
									<span className="rv-room">{r.room_code}</span>
									<span className="rv-date">{dayMonth(r.created_at)}</span>
								</div>
								{r.comment && <div className="rv-text">{r.comment}</div>}
							</div>
						))}
					</div>
				</>
			)}

			<p className="help">
				Số liệu ở đây để bạn theo dõi công việc. Lương và thưởng tính theo cơ chế riêng, không nằm trong phần mềm này.
			</p>
		</CleanerShell>
	)
}
