import { useContext } from 'react';
import { AuthContext } from '@/context/use-auth';
import CreateOrganizationPage from '@/pages/organizers/create/create';
import OrganizationDetailsForm from '@/pages/organizers/create/details';
import VerificationPendingPage from './verification';
import LoadingSkeleton from './loading';

// Auth Guard Component
const CreateGuard = ({ children }) => {
    const { activeOrganization, loading, hasLoadedOrganizations } = useContext(AuthContext);
  
    if (loading || !hasLoadedOrganizations) {
      return <LoadingSkeleton />;
    }
  
    if (!activeOrganization) {
      return <CreateOrganizationPage />;
    }
  
    if (!activeOrganization?.organization_verifications) {
      return <VerificationPendingPage orgId={activeOrganization?.id} />;
    }
  
    return children;
};
  
export default CreateGuard;