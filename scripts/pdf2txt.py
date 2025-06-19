#!/usr/bin/env python3
import pdftotext
import sys
import argparse
from pathlib import Path

def extract_text_from_pdf(pdf_path, password=None, output_file=None):
    """Extract text from PDF file"""
    try:
        with open(pdf_path, "rb") as f:
            if password:
                pdf = pdftotext.PDF(f, password)
            else:
                pdf = pdftotext.PDF(f)
        
        # Extract all text
        text = "\n\n".join(pdf)
        
        if output_file:
            with open(output_file, "w", encoding="utf-8") as f:
                f.write(text)
            print(f"Text extracted to {output_file}")
        else:
            print(text)
            
        return text
        
    except Exception as e:
        print(f"Error processing {pdf_path}: {e}", file=sys.stderr)
        return None

def main():
    parser = argparse.ArgumentParser(description="Extract text from PDF files")
    parser.add_argument("pdf_file", help="Path to PDF file")
    parser.add_argument("-p", "--password", help="PDF password if protected")
    parser.add_argument("-o", "--output", help="Output text file (optional)")
    parser.add_argument("--pages", help="Show page count", action="store_true")
    
    args = parser.parse_args()
    
    pdf_path = Path(args.pdf_file)
    if not pdf_path.exists():
        print(f"Error: File {pdf_path} not found", file=sys.stderr)
        sys.exit(1)
    
    if args.pages:
        try:
            with open(pdf_path, "rb") as f:
                pdf = pdftotext.PDF(f, args.password) if args.password else pdftotext.PDF(f)
            print(f"Pages: {len(pdf)}")
        except Exception as e:
            print(f"Error: {e}", file=sys.stderr)
            sys.exit(1)
    else:
        extract_text_from_pdf(pdf_path, args.password, args.output)

if __name__ == "__main__":
    main()