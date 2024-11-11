import React, { useContext } from 'react';
import { Card, CardHeader, CardTitle, CardDescription, CardContent, CardFooter } from '@/components/ui/card';
import { Button } from '@/components/ui/button';
import { PlusCircle, Ticket } from 'lucide-react';
import { AuthContext } from '@/context/use-auth';
import CreateOrganizationPage from './create';
import OrganizationDetailsForm from './details';

const LoadingSkeleton = () => (
  <div className="min-h-screen bg-gray-50 p-8 animate-pulse">
    {/* Header Section Skeleton */}
    <div className="max-w-6xl mx-auto mb-12">
      <div className="flex items-center space-x-4 mb-8">
        <div className="h-10 w-10 bg-gray-200 rounded-lg" />
        <div className="h-10 w-48 bg-gray-200 rounded-lg" />
      </div>
      <div className="h-6 w-2/3 bg-gray-200 rounded-lg" />
    </div>

    {/* Main Content Skeleton */}
    <div className="max-w-6xl mx-auto grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-6">
      {/* Create Ticket Card Skeleton */}
      <Card className="hover:shadow-lg transition-shadow">
        <CardHeader>
          <div className="flex items-center space-x-2">
            <div className="h-6 w-6 bg-gray-200 rounded-full" />
            <div className="h-6 w-32 bg-gray-200 rounded-lg" />
          </div>
          <div className="h-4 w-full bg-gray-200 rounded-lg mt-2" />
        </CardHeader>
        <CardContent>
          <div className="space-y-2">
            <div className="h-4 w-full bg-gray-200 rounded-lg" />
            <div className="h-4 w-3/4 bg-gray-200 rounded-lg" />
          </div>
        </CardContent>
        <CardFooter>
          <div className="h-10 w-full bg-gray-200 rounded-lg" />
        </CardFooter>
      </Card>

      {/* Stats Card Skeleton */}
      <Card>
        <CardHeader>
          <div className="h-6 w-32 bg-gray-200 rounded-lg" />
          <div className="h-4 w-40 bg-gray-200 rounded-lg" />
        </CardHeader>
        <CardContent>
          <div className="space-y-4">
            {[1, 2, 3].map((i) => (
              <div key={i} className="flex justify-between items-center">
                <div className="h-4 w-32 bg-gray-200 rounded-lg" />
                <div className="h-4 w-16 bg-gray-200 rounded-lg" />
              </div>
            ))}
          </div>
        </CardContent>
      </Card>

      {/* Recent Activity Card Skeleton */}
      <Card>
        <CardHeader>
          <div className="h-6 w-32 bg-gray-200 rounded-lg" />
          <div className="h-4 w-48 bg-gray-200 rounded-lg" />
        </CardHeader>
        <CardContent>
          <div className="space-y-4">
            {[1, 2, 3].map((i) => (
              <div key={i} className="border-l-4 border-gray-200 pl-4">
                <div className="h-4 w-40 bg-gray-200 rounded-lg mb-1" />
                <div className="h-3 w-24 bg-gray-200 rounded-lg" />
              </div>
            ))}
          </div>
        </CardContent>
      </Card>
    </div>
  </div>
);

const HomePage = () => {
  const { activeOrganization, loading, hasLoadedOrganizations } = useContext(AuthContext);

  if (loading || !hasLoadedOrganizations) {
    return <LoadingSkeleton />;
  }
  
  if (!activeOrganization) {
    return <CreateOrganizationPage />;
  }
  
  if (!activeOrganization?.organization_verifications) {
    return <OrganizationDetailsForm orgId={activeOrganization?.id}/>;
  }
  
  return (
    <div className="min-h-screen bg-gray-50 p-8">
      {/* Header Section */}
      <div className="max-w-6xl mx-auto mb-12">
        <div className="flex items-center space-x-4 mb-8">
          <Ticket className="h-10 w-10 text-primary" />
          <h1 className="text-4xl font-bold text-gray-900">Cackle Tickets</h1>
        </div>
        <p className="text-xl text-gray-600">Streamline your support workflow with our intuitive ticketing system</p>
      </div>

      {/* Main Content */}
      <div className="max-w-6xl mx-auto grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-6">
        {/* Create Ticket Card */}
        <Card className="hover:shadow-lg transition-shadow">
          <CardHeader>
            <CardTitle className="flex items-center space-x-2">
              <PlusCircle className="h-6 w-6" />
              <span>Create New Ticket</span>
            </CardTitle>
            <CardDescription>
              Submit a new support request or report an issue
            </CardDescription>
          </CardHeader>
          <CardContent>
            <p className="text-gray-600">
              Get quick assistance from our support team by creating a new ticket. 
              We typically respond within 24 hours.
            </p>
          </CardContent>
          <CardFooter>
            <Button className="w-full">
              Create Ticket
            </Button>
          </CardFooter>
        </Card>

        {/* Stats Card */}
        <Card>
          <CardHeader>
            <CardTitle>Quick Stats</CardTitle>
            <CardDescription>Current system status</CardDescription>
          </CardHeader>
          <CardContent>
            <div className="space-y-4">
              <div className="flex justify-between items-center">
                <span className="text-gray-600">Average Response Time</span>
                <span className="font-semibold">2.5 hours</span>
              </div>
              <div className="flex justify-between items-center">
                <span className="text-gray-600">Open Tickets</span>
                <span className="font-semibold">12</span>
              </div>
              <div className="flex justify-between items-center">
                <span className="text-gray-600">Resolved Today</span>
                <span className="font-semibold">45</span>
              </div>
            </div>
          </CardContent>
        </Card>

        {/* Recent Activity Card */}
        <Card>
          <CardHeader>
            <CardTitle>Recent Activity</CardTitle>
            <CardDescription>Latest updates from the platform</CardDescription>
          </CardHeader>
          <CardContent>
            <div className="space-y-4">
              <div className="border-l-4 border-green-500 pl-4">
                <p className="text-sm text-gray-600">Ticket #1234 resolved</p>
                <p className="text-xs text-gray-400">5 minutes ago</p>
              </div>
              <div className="border-l-4 border-blue-500 pl-4">
                <p className="text-sm text-gray-600">New ticket created</p>
                <p className="text-xs text-gray-400">15 minutes ago</p>
              </div>
              <div className="border-l-4 border-yellow-500 pl-4">
                <p className="text-sm text-gray-600">Ticket #1233 updated</p>
                <p className="text-xs text-gray-400">1 hour ago</p>
              </div>
            </div>
          </CardContent>
        </Card>
      </div>
    </div>
  );
};

export default HomePage;