import { createContext } from 'react';
import { boolean } from 'zod';

// Create the AuthContext with a default value
export const AuthContext = createContext({
  loading: true,
  user: null,
  hasLoadedOrganizations: boolean,
  signUp: async () => {},
  signIn: async () => {},
  signInWithGoogle: async () => {},
  signOut: async () => {},
  forgotPassword: async () => {},
  updateUserPassword: async () => {},
});

export default AuthContext;