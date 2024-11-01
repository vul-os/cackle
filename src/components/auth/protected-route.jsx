import React, { useContext, useEffect, useRef } from 'react';
import { useLocation, useNavigate, useSearchParams } from 'react-router-dom';
import { Progress } from "@/components/ui/progress";
import { AuthContext } from '../../context/use-auth';

const REDIRECT_STORAGE_KEY = 'auth_redirect_data';

const ProtectedRoute = ({ children }) => {
  const { user, loading } = useContext(AuthContext);
  const location = useLocation();
  const navigate = useNavigate();
  const [searchParams] = useSearchParams();
  const isMounted = useRef(false);

  useEffect(() => {
    if (!loading && isMounted.current) {
      const token = searchParams.get('token');
      const currentPath = location.pathname;
      const currentSearch = location.search;

      if (!user) {
        // Store token and return URL if we're not already on login page
        if (currentPath !== '/login') {
          const redirectData = {
            token,
            returnUrl: currentPath + currentSearch
          };
          localStorage.setItem(REDIRECT_STORAGE_KEY, JSON.stringify(redirectData));
          navigate('/login');
        }
      } else {
        // Check for stored redirect data first
        const storedData = localStorage.getItem(REDIRECT_STORAGE_KEY);
        if (storedData) {
          const { token: storedToken } = JSON.parse(storedData);
          if (storedToken) {
            localStorage.removeItem(REDIRECT_STORAGE_KEY);
            navigate(`/accept-invite?token=${storedToken}`);
            return;
          }
        }
        
        // If no stored data but we have a token in URL, handle it
        if (token && currentPath !== '/accept-invite') {
          navigate(`/accept-invite?token=${token}`);
        }
      }
    }
    isMounted.current = true;
  }, [user, loading]);

  if (loading) {
    return (
      <div className="w-full max-w-md mx-auto mt-8">
        <Progress value={33} className="w-full" />
      </div>
    );
  }

  return user ? children : null;
};

export default ProtectedRoute;