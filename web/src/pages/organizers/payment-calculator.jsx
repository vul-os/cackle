import React, { useState } from 'react';
import { Calculator, Percent, CreditCard as CreditCardIcon, Banknote as BanknoteIcon, Wallet as WalletIcon } from 'lucide-react';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';

const VAT_RATE = 0.15;
const OUR_FEE_RATE = 0.0085;

const PAYMENT_METHODS = {
    card: {
        name: 'Card Payments',
        icon: <CreditCardIcon className="h-4 w-4 text-primary" />,
        feeCalculation: (tickets, price) => tickets * price * 0.029 + tickets * 1,
    },
    cash: {
        name: 'Cash Payments',
        icon: <BanknoteIcon className="h-4 w-4 text-primary" />,
        feeCalculation: (tickets, price) => tickets * price * 0.039 + tickets * 6,
    },
    payshap: {
        name: 'PayShap',
        icon: <WalletIcon className="h-4 w-4 text-primary" />,
        feeCalculation: (tickets) => tickets * 7.5,
    },
};

const PaymentCalculator = () => {
    const [numTickets, setNumTickets] = useState(100);
    const [ticketPrice, setTicketPrice] = useState(100);
    const [includeVat, setIncludeVat] = useState(true);
    const [distribution, setDistribution] = useState({ card: 90, cash: 8, payshap: 2 });

    const handleDistributionChange = (method, value) => {
        const newValue = Math.max(0, Math.min(100, parseInt(value, 10) || 0));
        const others = Object.entries(distribution).filter(([key]) => key !== method);
        const othersTotal = others.reduce((sum, [, val]) => sum + val, 0);

        const newDist = { ...distribution, [method]: newValue };
        if (othersTotal > 0) {
            const scale = (100 - newValue) / othersTotal;
            others.forEach(([key, val]) => {
                newDist[key] = Math.round(val * scale);
            });
            const total = Object.values(newDist).reduce((sum, v) => sum + v, 0);
            if (total !== 100 && others.length) newDist[others[others.length - 1][0]] += 100 - total;
        }
        setDistribution(newDist);
    };

    const distributionData = Object.entries(distribution).map(([method, percentage]) => {
        const tickets = Math.round(numTickets * (percentage / 100));
        const config = PAYMENT_METHODS[method];
        return { id: method, name: config.name, icon: config.icon, percentage, tickets, fees: config.feeCalculation(tickets, ticketPrice) };
    });

    const ourFee = numTickets * ticketPrice * OUR_FEE_RATE;
    const methodFees = distributionData.reduce((sum, m) => sum + m.fees, 0);
    const subtotal = ourFee + methodFees;
    const vat = includeVat ? subtotal * VAT_RATE : 0;
    const total = subtotal + vat;

    return (
        <div className="mb-16">
            <div className="mb-12 text-center">
                <span className="mb-4 block text-sm font-semibold uppercase tracking-wider text-primary">Fee Calculator</span>
                <h2 className="mb-4 font-display text-3xl font-bold tracking-tight">Estimate your fees</h2>
                <p className="mx-auto max-w-2xl text-muted-foreground">See what a real event would cost, transparently.</p>
            </div>

            <div className="grid gap-8 md:grid-cols-2">
                <Card>
                    <CardHeader>
                        <CardTitle className="flex items-center gap-2 text-base">
                            <Calculator className="h-5 w-5 text-primary" />
                            Event Details
                        </CardTitle>
                    </CardHeader>
                    <CardContent className="space-y-6">
                        <div className="space-y-2">
                            <Label>Number of tickets</Label>
                            <Input type="number" value={numTickets} onChange={(e) => setNumTickets(Math.max(0, parseInt(e.target.value, 10) || 0))} />
                        </div>
                        <div className="space-y-2">
                            <Label>Ticket price (R)</Label>
                            <Input type="number" value={ticketPrice} onChange={(e) => setTicketPrice(Math.max(0, parseInt(e.target.value, 10) || 0))} />
                        </div>
                        <label className="flex items-center gap-2 text-sm">
                            <input type="checkbox" checked={includeVat} onChange={(e) => setIncludeVat(e.target.checked)} className="accent-primary" />
                            Include VAT (15%)
                        </label>
                    </CardContent>
                </Card>

                <Card>
                    <CardHeader>
                        <CardTitle className="flex items-center gap-2 text-base">
                            <Percent className="h-5 w-5 text-primary" />
                            Payment Distribution
                        </CardTitle>
                    </CardHeader>
                    <CardContent className="space-y-6">
                        {distributionData.map(({ id, name, icon, percentage, tickets }) => (
                            <div key={id} className="space-y-2">
                                <div className="flex items-center justify-between">
                                    <div className="flex items-center gap-2 text-sm font-medium">
                                        {icon}
                                        {name}
                                    </div>
                                    <span className="text-sm font-bold">{percentage}%</span>
                                </div>
                                <input
                                    type="range"
                                    min="0"
                                    max="100"
                                    value={percentage}
                                    onChange={(e) => handleDistributionChange(id, e.target.value)}
                                    className="h-2 w-full cursor-pointer accent-primary"
                                />
                                <div className="text-right text-sm text-muted-foreground">{tickets} tickets</div>
                            </div>
                        ))}
                    </CardContent>
                </Card>
            </div>

            <Card className="mt-8">
                <CardHeader>
                    <CardTitle>Fee breakdown</CardTitle>
                </CardHeader>
                <CardContent className="space-y-4">
                    <div className="grid grid-cols-2 gap-4">
                        <div className="rounded-lg bg-primary/10 p-4">
                            <h4 className="mb-1 text-sm font-medium text-muted-foreground">Our Fee (0.85%)</h4>
                            <p className="text-2xl font-bold text-primary">R{ourFee.toFixed(2)}</p>
                        </div>
                        {distributionData.map((m) => (
                            <div key={m.id} className="rounded-lg bg-muted p-4">
                                <h4 className="mb-1 text-sm font-medium text-muted-foreground">{m.name} Fees</h4>
                                <p className="text-2xl font-bold">R{m.fees.toFixed(2)}</p>
                            </div>
                        ))}
                    </div>

                    <div className="space-y-2 border-t border-border pt-4">
                        <div className="flex items-center justify-between">
                            <span className="text-muted-foreground">Subtotal</span>
                            <span className="font-bold">R{subtotal.toFixed(2)}</span>
                        </div>
                        <div className="flex items-center justify-between">
                            <span className="text-muted-foreground">VAT (15%)</span>
                            <span className="font-bold">R{vat.toFixed(2)}</span>
                        </div>
                        <div className="flex items-center justify-between border-t border-border pt-2">
                            <span className="text-lg font-bold">Total Fees</span>
                            <span className="text-lg font-bold text-primary">R{total.toFixed(2)}</span>
                        </div>
                    </div>
                </CardContent>
            </Card>

            <div className="mt-8 text-center text-sm text-muted-foreground">
                <p>Estimates only — actual fees depend on final transaction volumes and payment methods used.</p>
            </div>
        </div>
    );
};

export default PaymentCalculator;
