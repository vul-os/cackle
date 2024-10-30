"use client";

import React, { useState } from 'react';
import { useForm } from "react-hook-form";
import { Button } from '@/components/ui/button';
import {
  Form,
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from "@/components/ui/form";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";
import DatePickerWithRange from '@/components/date-range-picker';

const TicketTypeForm = ({ 
  initialData = null, 
  onSubmit, 
  isSubmitting = false 
}) => {
  const [date, setDate] = useState(() => {
    if (initialData?.sale_start_time && initialData?.sale_end_time) {
      return {
        from: new Date(initialData.sale_start_time),
        to: new Date(initialData.sale_end_time)
      };
    }
    return null;
  });

  const form = useForm({
    defaultValues: {
      name: initialData?.name || '',
      description: initialData?.description || '',
      price: initialData?.price?.toString() || '',
      quantity_total: initialData?.quantity_total?.toString() || '',
    }
  });

  const handleSubmit = (formData) => {
    if (!date?.from || !date?.to) {
      form.setError('root', {
        type: 'manual',
        message: 'Please select a date range'
      });
      return;
    }
    
    const submissionData = {
      ...formData,
      price: parseFloat(formData.price),
      quantity_total: parseInt(formData.quantity_total),
      sale_start_time: date.from.toISOString(),
      sale_end_time: date.to.toISOString(),
    };
    
    onSubmit(submissionData);
  };

  return (
    <Form {...form}>
      <form onSubmit={form.handleSubmit(handleSubmit)} className="space-y-4">
        <FormField
          control={form.control}
          name="name"
          render={({ field }) => (
            <FormItem>
              <FormLabel>Name</FormLabel>
              <FormControl>
                <Input 
                  {...field} 
                  required
                  placeholder="Enter ticket type name"
                />
              </FormControl>
              <FormMessage />
            </FormItem>
          )}
        />

        <FormField
          control={form.control}
          name="description"
          render={({ field }) => (
            <FormItem>
              <FormLabel>Description</FormLabel>
              <FormControl>
                <Textarea 
                  {...field} 
                  placeholder="Enter ticket description"
                />
              </FormControl>
              <FormMessage />
            </FormItem>
          )}
        />

        <FormField
          control={form.control}
          name="price"
          render={({ field }) => (
            <FormItem>
              <FormLabel>Price</FormLabel>
              <FormControl>
                <Input 
                  type="number"
                  step="0.01"
                  required
                  placeholder="0.00"
                  {...field}
                  onChange={(e) => {
                    const value = e.target.value;
                    if (value === '' || /^\d*\.?\d{0,2}$/.test(value)) {
                      field.onChange(value);
                    }
                  }}
                />
              </FormControl>
              <FormMessage />
            </FormItem>
          )}
        />

        <FormField
          control={form.control}
          name="quantity_total"
          render={({ field }) => (
            <FormItem>
              <FormLabel>Total Quantity</FormLabel>
              <FormControl>
                <Input 
                  type="number"
                  required
                  placeholder="0"
                  {...field}
                  onChange={(e) => {
                    const value = e.target.value;
                    if (value === '' || /^\d+$/.test(value)) {
                      field.onChange(value);
                    }
                  }}
                />
              </FormControl>
              <FormMessage />
            </FormItem>
          )}
        />

        <FormItem>
          <FormLabel>Sale Period</FormLabel>
          <DatePickerWithRange
            date={date}
            setDate={setDate}
            className="w-full"
          />
          {!initialData && (
            <p className="text-sm text-muted-foreground mt-1">
              Select a date range for ticket sales
            </p>
          )}
        </FormItem>

        {form.formState.errors.root && (
          <p className="text-sm text-red-500">
            {form.formState.errors.root.message}
          </p>
        )}

        <Button 
          type="submit" 
          className="w-full" 
          disabled={isSubmitting}
        >
          {isSubmitting ? 'Saving...' : (initialData ? 'Update Ticket Type' : 'Create Ticket Type')}
        </Button>
      </form>
    </Form>
  );
};

export default TicketTypeForm;