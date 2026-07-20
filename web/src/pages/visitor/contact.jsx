import React, { useState } from 'react';
import { Button } from '@/components/ui/button';
import { Card, CardHeader, CardTitle, CardContent } from '@/components/ui/card';
import { Input } from '@/components/ui/input';
import { Textarea } from '@/components/ui/textarea';
import { BookOpen, Phone, Mail } from 'lucide-react';
import Footer from '@/pages/visitor/landing/footer.jsx';
import Header from '@/pages/visitor/header.jsx';
import { toast } from '@/components/ui/use-toast';

const ContactIconBlock = ({ icon: Icon, title, description, link, linkText }) => (
    <div className="flex gap-x-6 py-6">
        <Icon className="mt-1 h-6 w-6 flex-shrink-0 text-primary" />
        <div className="grow">
            <h3 className="font-bold">{title}</h3>
            <p className="mt-1 text-sm text-muted-foreground">{description}</p>
            {link && (
                <a className="mt-2 inline-flex items-center gap-x-2 text-sm font-medium text-primary hover:underline" href={link}>
                    {linkText}
                </a>
            )}
        </div>
    </div>
);

const ContactPage = () => {
    const [formData, setFormData] = useState({ firstName: '', lastName: '', email: '', phone: '', details: '' });

    const handleChange = (e) => {
        const { name, value } = e.target;
        setFormData((prev) => ({ ...prev, [name]: value }));
    };

    const handleSubmit = (e) => {
        e.preventDefault();
        toast({ title: 'Thanks!', description: "We've received your message and will reply within 1-2 business days." });
        setFormData({ firstName: '', lastName: '', email: '', phone: '', details: '' });
    };

    return (
        <>
            <Header />
            <main className="min-h-screen bg-background pt-16">
                <section className="mx-auto max-w-5xl px-4 py-12">
                    <div className="mb-12 text-center">
                        <span className="mb-4 block text-sm font-semibold uppercase tracking-wider text-primary">Get in Touch</span>
                        <h1 className="mb-4 font-display text-4xl font-bold">Contact Us</h1>
                        <p className="mx-auto max-w-2xl text-muted-foreground">Questions about an event, a refund, or selling tickets? We're here.</p>
                    </div>

                    <div className="grid items-start gap-6 lg:grid-cols-2 lg:gap-16">
                        <Card>
                            <CardHeader>
                                <CardTitle>Send us a message</CardTitle>
                            </CardHeader>
                            <CardContent>
                                <form onSubmit={handleSubmit} className="grid gap-4">
                                    <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
                                        <Input name="firstName" value={formData.firstName} onChange={handleChange} placeholder="First Name" />
                                        <Input name="lastName" value={formData.lastName} onChange={handleChange} placeholder="Last Name" />
                                    </div>
                                    <Input type="email" name="email" value={formData.email} onChange={handleChange} placeholder="Email" required />
                                    <Input type="tel" name="phone" value={formData.phone} onChange={handleChange} placeholder="Phone Number" />
                                    <Textarea name="details" value={formData.details} onChange={handleChange} rows={4} placeholder="Details" required />
                                    <Button type="submit" className="w-full">
                                        Send Message
                                    </Button>
                                    <p className="text-center text-sm text-muted-foreground">We&apos;ll get back to you in 1-2 business days.</p>
                                </form>
                            </CardContent>
                        </Card>

                        <div className="divide-y divide-border">
                            <ContactIconBlock icon={BookOpen} title="Docs" description="Read the documentation." link="/docs" linkText="Browse docs" />
                            <ContactIconBlock icon={Phone} title="Call us" description="Have an urgent question?" link="tel:0674358901" linkText="067 435 8901" />
                            <ContactIconBlock icon={Mail} title="Email us" description="Prefer writing?" link="mailto:hello@cackle.app" linkText="hello@cackle.app" />
                        </div>
                    </div>
                </section>
            </main>
            <Footer />
        </>
    );
};

export default ContactPage;
