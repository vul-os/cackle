import { createContext } from 'react';

// Create the AuthContext with a default value
export const AuthContext = createContext({
  loading: true,
  user: null,
  signUp: async () => {},
  signIn: async () => {},
  signInWithGoogle: async () => {},
  signOut: async () => {},
  forgotPassword: async () => {},
  updateUserPassword: async () => {},
});

export default AuthContext;