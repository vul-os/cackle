import { Card, CardHeader, CardTitle, CardDescription, CardContent, CardFooter } from '@/components/ui/card';

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

  export default LoadingSkeleton