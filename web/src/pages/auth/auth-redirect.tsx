import { useLocation, useNavigate } from 'react-router-dom';

export const REDIRECT_STORAGE_KEY = 'auth_redirect_data';

// Two independent things redirect a signed-out visitor to /login and expect
// to land back where they were: ProtectedRoute + the cart's own checkout
// guard (both write REDIRECT_STORAGE_KEY to localStorage), and the 401
// handler in AuthProvider (which instead passes router state, since it
// fires mid-navigation). Check router state first — it's the more specific,
// freshest signal — then fall back to the persisted localStorage path.
export const useAuthRedirect = () => {
  const navigate = useNavigate();
  const location = useLocation();

  const handleSuccessfulAuth = () => {
    const stateReturnTo = location.state?.returnTo;
    const storedPath = (() => {
      try {
        return localStorage.getItem(REDIRECT_STORAGE_KEY);
      } catch {
        return null;
      }
    })();

    const redirectPath = stateReturnTo || storedPath;
    try {
      localStorage.removeItem(REDIRECT_STORAGE_KEY);
    } catch {
      // best-effort cleanup only
    }

    navigate(redirectPath || '/');
  };

  return handleSuccessfulAuth;
};