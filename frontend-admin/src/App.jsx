// Định tuyến toàn app + chốt chặn theo vai.
//
// Quy tắc: quản lý vào thẳng bảng điều phối, cô dọn dẹp vào thẳng ca hôm nay.
// Không ai thấy màn của vai kia — và nếu có gõ tay URL thì cũng bị đẩy về đúng
// chỗ của mình. Backend vẫn kiểm quyền độc lập; đây chỉ là điều hướng.

import { useEffect } from 'react'
import { AuthProvider, isAdmin, useAuth } from './auth.jsx'
import { RouterProvider, matchPath, useRouter } from './router.jsx'
import { LoginPage, RegisterPage } from './LoginPage.jsx'
import { Spinner } from './components/ui.jsx'
import { CleanerMePage, CleanerSessionPage, CleanerTodayPage } from './cleaner/CleanerPages.jsx'
import {
	BoardPage,
	ChecklistPage,
	ReportPage,
	ReviewPage,
	ReviewsPage,
	RoomsPage,
	SessionDetailPage,
	StaffPage,
} from './admin/AdminPages.jsx'

const PUBLIC = ['/login', '/register']

function Routes() {
	const { user, loading } = useAuth()
	const { path, navigate } = useRouter()

	useEffect(() => {
		if (loading) return
		const isPublic = PUBLIC.includes(path)
		if (!user && !isPublic) {
			navigate('/login', { replace: true })
			return
		}
		if (user && isPublic) {
			navigate(isAdmin(user) ? '/' : '/cleaning', { replace: true })
		}
	}, [user, loading, path, navigate])

	if (loading) return <Spinner />

	if (path === '/login') return <LoginPage />
	if (path === '/register') return <RegisterPage />
	if (!user) return <Spinner />

	// ── Màn của cô dọn dẹp ──
	const cleanerSession = matchPath('/cleaning/session/:id', path)
	if (cleanerSession) return <CleanerSessionPage sessionId={cleanerSession.id} />
	if (path === '/cleaning/me') return <CleanerMePage />
	if (path === '/cleaning') return <CleanerTodayPage />

	// Cô dọn dẹp gõ tay URL của quản lý → đẩy về ca hôm nay.
	if (!isAdmin(user)) {
		return <Redirect to="/cleaning" />
	}

	// ── Màn quản lý ──
	const adminSession = matchPath('/sessions/:id', path)
	if (adminSession) return <SessionDetailPage sessionId={adminSession.id} />
	if (path === '/review') return <ReviewPage />
	if (path === '/report') return <ReportPage />
	if (path === '/reviews') return <ReviewsPage />
	if (path === '/staff') return <StaffPage />
	if (path === '/rooms') return <RoomsPage />
	if (path === '/checklists') return <ChecklistPage />
	if (path === '/') return <BoardPage />

	return <Redirect to="/" />
}

function Redirect({ to }) {
	const { navigate } = useRouter()
	useEffect(() => {
		navigate(to, { replace: true })
	}, [to, navigate])
	return <Spinner />
}

export default function App() {
	return (
		<RouterProvider>
			<AuthProvider>
				<Routes />
			</AuthProvider>
		</RouterProvider>
	)
}
