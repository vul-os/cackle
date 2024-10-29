import { Head } from '@/components/head';
import Navigation from '@/components/navigation';

export default function AppLayout({ children, title }) {
  return (
    <>
      <Head title={title} />
      <Navigation />
      <div className="flex">
        <div className="flex-grow mt-[70px] ml-[250px] p-4">
          {/* Adjust margin-top and margin-left to avoid overlay */}
          <main>{children}</main>
        </div>
      </div>
    </>
  );
}
