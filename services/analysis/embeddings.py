#!/usr/bin/env python3
"""
Embedding Generator for Market Intelligence Aggregator.

Generates vector embeddings for creator content using sentence-transformers
and stores them in the database for semantic search.

Usage:
    python embeddings.py [--batch-size N] [--limit N]

Environment Variables Required:
    DATABASE_URL: PostgreSQL connection string
"""

import os
import sys
import json
import argparse
from datetime import datetime

try:
    import psycopg
    from sentence_transformers import SentenceTransformer
except ImportError as e:
    print(json.dumps({
        'status': 'error',
        'message': f'Missing dependency: {e}. Run: pip install -r requirements.txt'
    }))
    sys.exit(1)


# Model configuration
MODEL_NAME = 'all-MiniLM-L6-v2'
EMBEDDING_DIMENSION = 384  # Must match database schema: vector(384)


def get_database_connection():
    """Create database connection."""
    db_url = os.getenv('DATABASE_URL')
    if not db_url:
        raise ValueError("DATABASE_URL environment variable is not set")
    return psycopg.connect(db_url)


def load_model() -> SentenceTransformer:
    """Load the sentence-transformers model.
    
    Returns:
        Loaded SentenceTransformer model
    """
    print(f"Loading model: {MODEL_NAME}...", file=sys.stderr)
    model = SentenceTransformer(MODEL_NAME)
    print(f"Model loaded successfully", file=sys.stderr)
    return model


def fetch_content_without_embeddings(limit: int = 100) -> list[tuple]:
    """Fetch creator content that doesn't have embeddings yet.
    
    Args:
        limit: Maximum number of rows to fetch
        
    Returns:
        List of (id, content_text) tuples
    """
    with get_database_connection() as conn:
        with conn.cursor() as cur:
            cur.execute("""
                SELECT id, content_text
                FROM creator_content
                WHERE embedding IS NULL
                ORDER BY created_at DESC
                LIMIT %s
            """, (limit,))
            
            return cur.fetchall()


def store_embedding(content_id: int, embedding: list[float]) -> bool:
    """Store embedding for a content item.
    
    Args:
        content_id: ID of the content row
        embedding: List of floats representing the embedding vector
        
    Returns:
        True if storage successful
    """
    if len(embedding) != EMBEDDING_DIMENSION:
        print(f"Warning: Embedding dimension mismatch. Expected {EMBEDDING_DIMENSION}, got {len(embedding)}", 
              file=sys.stderr)
        return False
    
    try:
        with get_database_connection() as conn:
            with conn.cursor() as cur:
                # Format embedding as PostgreSQL vector
                embedding_str = '[' + ','.join(str(x) for x in embedding) + ']'
                
                cur.execute("""
                    UPDATE creator_content
                    SET embedding = %s::vector
                    WHERE id = %s
                """, (embedding_str, content_id))
                
                conn.commit()
                return True
        
    except Exception as e:
        print(f"Error storing embedding for id {content_id}: {e}", file=sys.stderr)
        return False


def generate_embeddings(model: SentenceTransformer, texts: list[str], batch_size: int = 32) -> list:
    """Generate embeddings for a list of texts.
    
    Args:
        model: SentenceTransformer model
        texts: List of text strings
        batch_size: Batch size for encoding
        
    Returns:
        List of embedding arrays
    """
    return model.encode(texts, batch_size=batch_size, show_progress_bar=True)


def main() -> None:
    """Main entry point for embedding generation."""
    parser = argparse.ArgumentParser(description='Generate embeddings for creator content')
    parser.add_argument('--batch-size', type=int, default=32, help='Batch size for encoding')
    parser.add_argument('--limit', type=int, default=100, help='Maximum number of items to process')
    args = parser.parse_args()
    
    try:
        # Load model
        model = load_model()
        
        # Fetch content without embeddings
        rows = fetch_content_without_embeddings(limit=args.limit)
        
        if not rows:
            result = {
                'status': 'success',
                'processed': 0,
                'message': 'No content found without embeddings',
                'timestamp': datetime.now().isoformat()
            }
            print(json.dumps(result, indent=2))
            return
        
        print(f"Processing {len(rows)} content items...", file=sys.stderr)
        
        # Extract IDs and texts
        content_ids = [row[0] for row in rows]
        texts = [row[1] for row in rows]
        
        # Generate embeddings
        embeddings = generate_embeddings(model, texts, batch_size=args.batch_size)
        
        # Store embeddings
        success_count = 0
        error_count = 0
        
        for content_id, embedding in zip(content_ids, embeddings):
            if store_embedding(content_id, embedding.tolist()):
                success_count += 1
            else:
                error_count += 1
        
        result = {
            'status': 'success' if error_count == 0 else 'partial',
            'processed': success_count,
            'errors': error_count,
            'model': MODEL_NAME,
            'dimension': EMBEDDING_DIMENSION,
            'timestamp': datetime.now().isoformat()
        }
        
        print(json.dumps(result, indent=2))
        
        if success_count > 0:
            print(f"✓ Generated {success_count} embeddings", file=sys.stderr)
        
    except Exception as e:
        print(json.dumps({
            'status': 'error',
            'message': str(e),
            'timestamp': datetime.now().isoformat()
        }), file=sys.stderr)
        sys.exit(1)


if __name__ == '__main__':
    main()
