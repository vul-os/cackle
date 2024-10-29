import React, { useEffect, useState, useContext } from 'react';
import { supabase } from '../services/supabaseClient';
import { AuthContext } from '../context/use-auth';
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Accordion, AccordionContent, AccordionItem, AccordionTrigger } from "@/components/ui/accordion";
import { Folder, ChevronRight, Upload } from 'lucide-react';
import { toast } from "@/components/ui/use-toast";

const Categories = () => {
  const [categories, setCategories] = useState([]);
  const { user } = useContext(AuthContext);

  useEffect(() => {
    if (user) {
      fetchCategories();
    }
  }, [user]);

  const fetchCategories = async () => {
    const { data, error } = await supabase
      .from('categories')
      .select(`
        id,
        name,
        description,
        subcategories (
          id,
          name,
          description
        )
      `)
      .eq('user_id', user.id)
      .order('name');

    if (error) {
      console.error('Error fetching categories:', error);
      toast({
        title: "Error",
        description: "Failed to fetch categories. Please try again.",
        variant: "destructive",
      });
    } else {
      setCategories(data);
    }
  };

  const EmptyState = () => (
    <div className="text-center py-8">
      <p className="text-lg mb-4">No categories found.</p>
      <p className="text-gray-600 mb-4">To get started, head over to the Slips section and process a group.</p>
      <Button variant="outline" onClick={() => navigate('/slips')}>
        Go to Slips
      </Button>
    </div>
  );

  return (
    <Card className="w-full">
      <CardHeader>
        <CardTitle>Categories</CardTitle>
      </CardHeader>
      <CardContent>
        {categories.length === 0 ? (
          <EmptyState />
        ) : (
          <Accordion type="single" collapsible className="w-full">
            {categories.map((category) => (
              <AccordionItem value={category.id} key={category.id}>
                <AccordionTrigger>
                  <div className="flex items-center justify-between w-full">
                    <div className="flex items-center">
                      <Folder className="mr-2 h-4 w-4" />
                      <span>{category.name}</span>
                    </div>
                  </div>
                </AccordionTrigger>
                <AccordionContent>
                  <div className="pl-6 space-y-2">
                    {category.subcategories.map((subcategory) => (
                      <div key={subcategory.id} className="flex items-center text-sm">
                        <ChevronRight className="mr-2 h-3 w-3" />
                        <span>{subcategory.name}</span>
                      </div>
                    ))}
                  </div>
                </AccordionContent>
              </AccordionItem>
            ))}
          </Accordion>
        )}
      </CardContent>
    </Card>
  );
};

export default Categories;