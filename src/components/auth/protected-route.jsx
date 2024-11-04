import React, { useContext, useEffect, useRef } from 'react';
import { useLocation, useNavigate } from 'react-router-dom';
import { Progress } from "@/components/ui/progress";
import { AuthContext } from '../../context/use-auth';

const REDIRECT_STORAGE_KEY = 'auth_redirect_data';

const ProtectedRoute = ({ children }) => {
  const { user, loading } = useContext(AuthContext);
  const location = useLocation();
  const navigate = useNavigate();
  const isMounted = useRef(false);

  useEffect(() => {
    if (!loading && isMounted.current) {
      const currentPath = location.pathname + location.search;
      console.log(currentPath)
      if (!user) {
        localStorage.setItem(REDIRECT_STORAGE_KEY, currentPath);
        navigate('/login');
      } else {
        const redirectPath = localStorage.getItem(REDIRECT_STORAGE_KEY);
        if (redirectPath) {
          localStorage.removeItem(REDIRECT_STORAGE_KEY);
          navigate(redirectPath);
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