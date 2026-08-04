import React from 'react';
import { Check } from 'lucide-react';
import { cn } from '@/lib/utils';

export interface WizardStep {
    key: string;
    label: string;
}

export interface WizardStepperProps {
    steps: WizardStep[];
    currentStep: number;
    maxStepReached: number;
    onStepClick: (index: number) => void;
}

/**
 * Horizontal step indicator for the create-event wizard. A step is
 * clickable once the wizard has reached it at least once (`maxStepReached`)
 * — you can always look back at a completed step, but you can't skip ahead
 * of what you've actually filled in.
 *
 * Two things carry the "where am I, what's left" orientation the wizard
 * needs: the "Step N of M" line (terse, always legible even once the pills
 * below it wrap across two lines on a narrow screen) and the pills
 * themselves, which name every remaining step rather than just a bare
 * count. `min-h-11` on the pill is deliberate, not decorative — it's the
 * suite's 44px touch-target floor, measured at 390px; a text-sm pill with
 * only `py-1.5` padding renders at ~32px and would fail that bar.
 */
const WizardStepper = ({ steps, currentStep, maxStepReached, onStepClick }: WizardStepperProps) => {
    return (
        <div className="mb-8">
            <p className="mb-3 text-xs font-medium uppercase tracking-wide text-muted-foreground">
                Step {currentStep + 1} of {steps.length} &middot; {steps[currentStep]?.label}
            </p>
            <ol className="flex flex-wrap items-center gap-x-2 gap-y-3" aria-label="Event creation steps">
                {steps.map((step, index) => {
                    const isComplete = index < maxStepReached;
                    const isCurrent = index === currentStep;
                    const isClickable = index <= maxStepReached;

                    return (
                        <li key={step.key} className="flex items-center">
                            <button
                                type="button"
                                onClick={() => isClickable && onStepClick(index)}
                                disabled={!isClickable}
                                aria-current={isCurrent ? 'step' : undefined}
                                className={cn(
                                    'flex min-h-11 items-center gap-2 rounded-full border px-3.5 py-1.5 text-sm font-medium transition-colors',
                                    isCurrent && 'border-primary bg-primary text-primary-foreground',
                                    !isCurrent && isComplete && 'border-primary/40 bg-primary/10 text-primary-emphasis hover:bg-primary/20',
                                    !isCurrent && !isComplete && 'border-border text-muted-foreground',
                                    !isClickable && 'cursor-not-allowed opacity-60',
                                )}
                            >
                                <span
                                    className={cn(
                                        'flex h-5 w-5 shrink-0 items-center justify-center rounded-full text-xs',
                                        isCurrent && 'bg-primary-foreground/20',
                                        !isCurrent && isComplete && 'bg-primary/20',
                                        !isCurrent && !isComplete && 'bg-muted',
                                    )}
                                >
                                    {isComplete ? <Check className="h-3 w-3" /> : index + 1}
                                </span>
                                {step.label}
                            </button>
                            {index < steps.length - 1 && <span className="mx-1 h-px w-4 shrink-0 bg-border sm:w-8" aria-hidden="true" />}
                        </li>
                    );
                })}
            </ol>
        </div>
    );
};

export default WizardStepper;
