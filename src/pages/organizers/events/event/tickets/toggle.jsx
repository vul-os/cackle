import * as React from "react";
import { Toggle } from "@/components/ui/toggle";
import { cn } from "@/lib/utils";

const TicketToggle = React.forwardRef((props, ref) => {
  const { 
    className, 
    isEnabled, 
    onToggleChange, 
    children, 
    ...restProps 
  } = props;

  return (
    <Toggle
      ref={ref}
      pressed={isEnabled}
      onPressedChange={onToggleChange}
      className={cn(
        "w-[200px] gap-2",
        isEnabled && "text-primary-foreground",
        className
      )}
      {...restProps}
    >
      <div className="flex items-center gap-2">
        {children || (isEnabled ? "Enabled" : "Disabled")}
      </div>
    </Toggle>
  )
});

TicketToggle.displayName = "TicketToggle";

export { TicketToggle };