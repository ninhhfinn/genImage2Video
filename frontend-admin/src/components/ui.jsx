// Mảnh giao diện dùng lại. Giữ nhỏ và không có logic nghiệp vụ.

import { SESSION_LABEL, STAFF_LABEL, ALLOWANCE_LABEL } from '../format.js'

const SESSION_TONE = {
	todo: 'neutral',
	in_progress: 'info',
	submitted: 'warn',
	approved: 'ok',
	rejected: 'danger',
}

const STAFF_TONE = { pending: 'warn', active: 'ok', suspended: 'neutral', rejected: 'danger' }
const ALLOWANCE_TONE = { pending: 'warn', approved: 'ok', rejected: 'danger' }

export function Badge({ tone = 'neutral', children }) {
	return <span className={`badge badge--${tone}`}>{children}</span>
}

export const SessionBadge = ({ status }) => (
	<Badge tone={SESSION_TONE[status]}>{SESSION_LABEL[status] || status}</Badge>
)

export const StaffBadge = ({ status }) => <Badge tone={STAFF_TONE[status]}>{STAFF_LABEL[status] || status}</Badge>

export const AllowanceBadge = ({ status }) => (
	<Badge tone={ALLOWANCE_TONE[status]}>{ALLOWANCE_LABEL[status] || status}</Badge>
)

export function Stat({ label, value, sub, tone }) {
	return (
		<div className={`stat${tone ? ` stat--${tone}` : ''}`}>
			<div className="stat-label">{label}</div>
			<div className="stat-value">{value}</div>
			{sub && <div className="stat-sub">{sub}</div>}
		</div>
	)
}

export function Alert({ tone = 'danger', children }) {
	if (!children) return null
	return <div className={`alert alert--${tone}`}>{children}</div>
}

export function Spinner() {
	return (
		<div className="loading">
			<span className="spin spin--lg" />
		</div>
	)
}

export function Empty({ icon = '📭', children }) {
	return (
		<div className="empty">
			<div className="empty-ico">{icon}</div>
			<div>{children}</div>
		</div>
	)
}

export function Progress({ percent }) {
	return (
		<div className="bar">
			<div className="bar-fill" style={{ width: `${Math.max(0, Math.min(100, percent || 0))}%` }} />
		</div>
	)
}
