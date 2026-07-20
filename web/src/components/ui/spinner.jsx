import { Loader2 } from "lucide-react";

export const Spinner = () => {
  return (
    <div className="fixed inset-0 flex items-center justify-center">
      <Loader2 className="h-16 w-16 animate-spin text-primary" />
    </div>
  );
};