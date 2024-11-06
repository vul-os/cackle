import React, { useState } from 'react';
import { Button } from "@/components/ui/button";
import { Card, CardHeader, CardTitle, CardContent } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";

const ContactIconBlock = ({ icon, title, description, address, link, linkText }) => (
  <div className="flex gap-x-7 py-6">
    {icon}
    <div className="grow">
      <h3 className="font-bold text-gray-800">
        {title}
      </h3>
      <p className="mt-1 text-sm text-gray-600">
        {description}
      </p>
      {address && (
        <p className="mt-1 text-sm italic text-gray-500">{address}</p>
      )}
      {link && (
        <a
          className="group mt-2 inline-flex items-center gap-x-2 rounded-lg text-sm font-medium text-red-800 outline-none ring-red-500 transition duration-300 hover:text-red-900 focus-visible:ring"
          href={link}
        >
          {linkText}
          {linkText !== "support@screwfast.uk" && (
            <svg
              className="h-4 w-4 flex-shrink-0 transition ease-in-out group-hover:translate-x-1"
              fill="none"
              viewBox="0 0 24 24"
              strokeWidth="1.5"
              stroke="currentColor"
            >
              <path
                strokeLinecap="round"
                strokeLinejoin="round"
                d="M13.5 4.5 21 12m0 0-7.5 7.5M21 12H3"
              />
            </svg>
          )}
        </a>
      )}
    </div>
  </div>
);

const ContactPage = () => {
  const [formData, setFormData] = useState({
    firstName: '',
    lastName: '',
    email: '',
    phone: '',
    details: ''
  });

  const handleChange = (e) => {
    const { name, value } = e.target;
    setFormData(prev => ({
      ...prev,
      [name]: value
    }));
  };

  const handleSubmit = (e) => {
    e.preventDefault();
    console.log(formData);
  };

  const icons = {
    knowledgeBase: (
      <svg
        className="mt-1.5 h-6 w-6 flex-shrink-0 text-red-800"
        viewBox="0 0 24 24"
        fill="none"
        stroke="currentColor"
        strokeWidth="1.5"
        strokeLinecap="round"
        strokeLinejoin="round"
      >
        <path d="M9.879 7.519c1.171-1.025 3.071-1.025 4.242 0 1.172 1.025 1.172 2.687 0 3.712-.203.179-.43.326-.67.442-.745.361-1.45.999-1.45 1.827v.75M21 12a9 9 0 1 1-18 0 9 9 0 0 1 18 0Zm-9 5.25h.008v.008H12v-.008Z" />
      </svg>
    ),
    faq: (
      <svg
        className="mt-1.5 h-6 w-6 flex-shrink-0 text-red-800"
        viewBox="0 0 24 24"
        fill="none"
        stroke="currentColor"
        strokeWidth="1.5"
        strokeLinecap="round"
        strokeLinejoin="round"
      >
        <path d="M20.25 8.511c.884.284 1.5 1.128 1.5 2.097v4.286c0 1.136-.847 2.1-1.98 2.193-.34.027-.68.052-1.02.072v3.091l-3-3c-1.354 0-2.694-.055-4.02-.163a2.115 2.115 0 0 1-.825-.242m9.345-8.334a2.126 2.126 0 0 0-.476-.095 48.64 48.64 0 0 0-8.048 0c-1.131.094-1.976 1.057-1.976 2.192v4.286c0 .837.46 1.58 1.155 1.951m9.345-8.334V6.637c0-1.621-1.152-3.026-2.76-3.235A48.455 48.455 0 0 0 11.25 3c-2.115 0-4.198.137-6.24.402-1.608.209-2.76 1.614-2.76 3.235v6.226c0 1.621 1.152 3.026 2.76 3.235.577.075 1.157.14 1.74.194V21l4.155-4.155" />
      </svg>
    ),
    location: (
      <svg
        className="mt-1.5 h-6 w-6 flex-shrink-0 text-red-800"
        viewBox="0 0 24 24"
        fill="none"
        stroke="currentColor"
        strokeWidth="1.5"
        strokeLinecap="round"
        strokeLinejoin="round"
      >
        <path d="M15 10.5a3 3 0 1 1-6 0 3 3 0 0 1 6 0Z" />
        <path d="M19.5 10.5c0 7.142-7.5 11.25-7.5 11.25S4.5 17.642 4.5 10.5a7.5 7.5 0 1 1 15 0Z" />
      </svg>
    ),
    email: (
      <svg
        className="mt-1.5 h-6 w-6 flex-shrink-0 text-red-800"
        viewBox="0 0 24 24"
        fill="none"
        stroke="currentColor"
        strokeWidth="1.5"
        strokeLinecap="round"
        strokeLinejoin="round"
      >
        <path d="M21.75 9v.906a2.25 2.25 0 0 1-1.183 1.981l-6.478 3.488M2.25 9v.906a2.25 2.25 0 0 0 1.183 1.981l6.478 3.488m8.839 2.51-4.66-2.51m0 0-1.023-.55a2.25 2.25 0 0 0-2.134 0l-1.022.55m0 0-4.661 2.51m16.5 1.615a2.25 2.25 0 0 1-2.25 2.25h-15a2.25 2.25 0 0 1-2.25-2.25V8.844a2.25 2.25 0 0 1 1.183-1.981l7.5-4.039a2.25 2.25 0 0 1 2.134 0l7.5 4.039a2.25 2.25 0 0 1 1.183 1.98V19.5Z" />
      </svg>
    ),
  };

  return (
    <div className="min-h-screen bg-gradient-to-br from-red-50 via-white to-red-50">
      <section className="mx-auto max-w-5xl px-4 py-12">
        <div className="text-center mb-12">
          <span className="text-red-600 font-semibold text-sm tracking-wider uppercase mb-4 block">Get in Touch</span>
          <h1 className="text-4xl font-bold mb-4 bg-gradient-to-r from-red-600 to-red-800 bg-clip-text text-transparent">
            Contact Us
          </h1>
          <p className="text-gray-600 max-w-2xl mx-auto">
            Turn every occasion into an experience. From vibrant virtual gatherings to sunset festivals and laid-back social meetups, there's an adventure waiting for everyone.
          </p>
        </div>

        <div className="grid items-center gap-6 lg:grid-cols-2 lg:gap-16">
          <Card className="flex flex-col rounded-xl p-4 sm:p-6 lg:p-8 bg-white/80 backdrop-blur-sm border-0 hover:shadow-xl transition-all duration-300">
            <CardHeader className="px-0">
              <CardTitle className="text-xl font-bold text-gray-800">
                Fill in the form below
              </CardTitle>
            </CardHeader>
            <CardContent className="px-0">
              <form onSubmit={handleSubmit} className="grid gap-4">
                <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
                  <div>
                    <label className="sr-only">First Name</label>
                    <Input
                      type="text"
                      name="firstName"
                      value={formData.firstName}
                      onChange={handleChange}
                      className="block w-full border-gray-200 focus:border-red-500 focus:ring-red-500"
                      placeholder="First Name"
                    />
                  </div>
                  <div>
                    <label className="sr-only">Last Name</label>
                    <Input
                      type="text"
                      name="lastName"
                      value={formData.lastName}
                      onChange={handleChange}
                      className="block w-full border-gray-200 focus:border-red-500 focus:ring-red-500"
                      placeholder="Last Name"
                    />
                  </div>
                </div>

                <div>
                  <label className="sr-only">Email</label>
                  <Input
                    type="email"
                    name="email"
                    value={formData.email}
                    onChange={handleChange}
                    className="block w-full border-gray-200 focus:border-red-500 focus:ring-red-500"
                    placeholder="Email"
                  />
                </div>

                <div>
                  <label className="sr-only">Phone Number</label>
                  <Input
                    type="tel"
                    name="phone"
                    value={formData.phone}
                    onChange={handleChange}
                    className="block w-full border-gray-200 focus:border-red-500 focus:ring-red-500"
                    placeholder="Phone Number"
                  />
                </div>

                <div>
                  <label className="sr-only">Details</label>
                  <Textarea
                    name="details"
                    value={formData.details}
                    onChange={handleChange}
                    rows={4}
                    className="block w-full border-gray-200 focus:border-red-500 focus:ring-red-500"
                    placeholder="Details"
                  />
                </div>

                <div className="mt-4">
                  <Button
                    type="submit"
                    className="inline-flex w-full items-center justify-center gap-x-2 rounded-lg bg-gradient-to-r from-red-900 to-red-700 px-4 py-3 text-sm font-bold text-white hover:from-red-950 hover:to-red-800 focus:ring-2 focus:ring-red-500"
                  >
                    Send Message
                  </Button>
                </div>

                <div className="mt-3 text-center">
                  <p className="text-sm text-gray-500">
                    We'll get back to you in 1-2 business days.
                  </p>
                </div>
              </form>
            </CardContent>
          </Card>

          <div className="divide-y divide-gray-200">
            <ContactIconBlock
              icon={icons.knowledgeBase}
              title="Knowledgebase"
              description="Browse through all of our knowledgebase articles."
              link="#"
              linkText="Visit guides & tutorials"
            />
            <ContactIconBlock
             icon={icons.phone}
              title="Contact Us"
                description="Have questions? Reach out to our support team directly."
                 link="tel:+1234567890"
                 linkText="0674358901"
         />
            <ContactIconBlock
              icon={icons.email}
              title="Contact us by email"
              description="Prefer the written word? Drop us an email at"
              link="#"
              linkText="cackleza@gmail.com"
            />
          </div>
        </div>
      </section>
    </div>
  );
};

export default ContactPage;