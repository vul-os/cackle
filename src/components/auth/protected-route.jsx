import React, { useContext, useEffect, useRef } from 'react';
import { useLocation, useNavigate } from 'react-router-dom';
import { Progress } from "@/components/ui/progress"
import { AuthContext } from '../../context/use-auth';

const ProtectedRoute = ({ children }) => {
  const { user, loading } = useContext(AuthContext);
  const location = useLocation();
  const navigate = useNavigate();
  const isMounted = useRef(false);

  useEffect(() => {
    if (!user && !loading && isMounted.current) {
      console.log("No User", location.pathname);
      
      // Save the current path to localStorage
      localStorage.setItem('redirectUrl', location.pathname);
      
      navigate('/login');
    }
    isMounted.current = true;
  }, [user, loading, navigate, location]);

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