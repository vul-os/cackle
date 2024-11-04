import { useNavigate } from 'react-router-dom';

export const REDIRECT_STORAGE_KEY = 'auth_redirect_data';

export const useAuthRedirect = () => {
  const navigate = useNavigate();
  
  const handleSuccessfulAuth = () => {
    const redirectPath = localStorage.getItem(REDIRECT_STORAGE_KEY);
    if (redirectPath) {
      localStorage.removeItem(REDIRECT_STORAGE_KEY);
      navigate(redirectPath);
    } else {
      navigate('/');
    }
  };

  return handleSuccessfulAuth;
};