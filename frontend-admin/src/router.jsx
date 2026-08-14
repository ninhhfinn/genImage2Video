// Router tối giản (~60 dòng) thay vì kéo react-router vào.
//
// App này có 9 màn, không lồng route, không cần loader/action. Một thư viện định
// tuyến ở đây là thêm một thứ phải nâng cấp theo React 19 để đổi lấy chức năng
// mà 60 dòng làm xong.

import { createContext, useContext, useEffect, useState, useCallback } from 'react'

const RouterContext = createContext({ path: '/', navigate: () => {} })

// Trên môi trường dev/local, app được phục vụ dưới /admin (không có subdomain).
// Cắt tiền tố đó đi để mọi nơi khác trong app chỉ nghĩ bằng đường dẫn "sạch".
const BASE = '/admin'

function stripBase(pathname) {
	if (pathname === BASE) return '/'
	if (pathname.startsWith(BASE + '/')) return pathname.slice(BASE.length)
	return pathname || '/'
}

function withBase(path) {
	if (typeof window !== 'undefined' && window.location.pathname.startsWith(BASE)) {
		return BASE + (path === '/' ? '' : path)
	}
	return path
}

export function RouterProvider({ children }) {
	const [path, setPath] = useState(() => stripBase(window.location.pathname))

	useEffect(() => {
		const onPop = () => setPath(stripBase(window.location.pathname))
		window.addEventListener('popstate', onPop)
		return () => window.removeEventListener('popstate', onPop)
	}, [])

	const navigate = useCallback((to, { replace = false } = {}) => {
		const full = withBase(to)
		if (replace) window.history.replaceState({}, '', full)
		else window.history.pushState({}, '', full)
		setPath(stripBase(window.location.pathname))
		window.scrollTo(0, 0)
	}, [])

	return <RouterContext.Provider value={{ path, navigate }}>{children}</RouterContext.Provider>
}

export function useRouter() {
	return useContext(RouterContext)
}

/** Link nội bộ — giữ được Ctrl/Cmd+click mở tab mới như thẻ <a> thật. */
export function Link({ to, children, ...rest }) {
	const { navigate } = useRouter()
	return (
		<a
			href={withBase(to)}
			onClick={(e) => {
				if (e.metaKey || e.ctrlKey || e.shiftKey || e.button !== 0) return
				e.preventDefault()
				navigate(to)
			}}
			{...rest}
		>
			{children}
		</a>
	)
}

/**
 * Khớp đường dẫn với mẫu có tham số: matchPath('/s/:id', '/s/abc') → {id:'abc'}.
 * Trả null khi không khớp.
 */
export function matchPath(pattern, path) {
	const pp = pattern.split('/').filter(Boolean)
	const ap = path.split('/').filter(Boolean)
	if (pp.length !== ap.length) return null
	const params = {}
	for (let i = 0; i < pp.length; i++) {
		if (pp[i].startsWith(':')) {
			params[pp[i].slice(1)] = decodeURIComponent(ap[i])
			continue
		}
		if (pp[i] !== ap[i]) return null
	}
	return params
}
