#!/usr/bin/env python3
"""
Technical Indicators Calculator for Market Intelligence Aggregator.

Calculates technical indicators (RSI, SMA, MACD, ATR) using pandas-ta
and stores results in the database.

Usage:
    python indicators.py [ticker1,ticker2,...]

Environment Variables Required:
    DATABASE_URL: PostgreSQL connection string
    TRACKED_TICKERS: (Optional) Comma-separated list of tickers
"""

import os
import sys
import json
from datetime import datetime, timedelta

try:
    import pandas as pd
    import pandas_ta as ta
    import psycopg
except ImportError as e:
    print(json.dumps({
        'status': 'error',
        'message': f'Missing dependency: {e}. Run: pip install -r requirements.txt'
    }))
    sys.exit(1)


def get_database_connection():
    """Create database connection."""
    db_url = os.getenv('DATABASE_URL')
    if not db_url:
        raise ValueError("DATABASE_URL environment variable is not set")
    return psycopg.connect(db_url)


def fetch_market_data(ticker: str, days: int = 200) -> pd.DataFrame:
    """Fetch historical market data for indicator calculation.
    
    Args:
        ticker: Stock ticker symbol
        days: Number of days of historical data to fetch
        
    Returns:
        DataFrame with OHLCV data
    """
    conn = get_database_connection()
    
    query = """
        SELECT timestamp, open, high, low, close, volume
        FROM market_data
        WHERE ticker = %s AND timestamp >= %s
        ORDER BY timestamp ASC
    """
    
    cutoff_date = datetime.now() - timedelta(days=days)
    
    try:
        with conn.cursor() as cur:
            cur.execute(query, (ticker, cutoff_date))
            rows = cur.fetchall()
            
            if not rows:
                return pd.DataFrame()
            
            df = pd.DataFrame(rows, columns=['timestamp', 'open', 'high', 'low', 'close', 'volume'])
            df['timestamp'] = pd.to_datetime(df['timestamp'])
            return df
    finally:
        conn.close()


def calculate_indicators(df: pd.DataFrame) -> pd.DataFrame:
    """Calculate all technical indicators.
    
    Args:
        df: DataFrame with OHLCV data
        
    Returns:
        DataFrame with added indicator columns
    """
    if df.empty or len(df) < 14:
        return df
    
    # RSI (14-day)
    df['rsi_14'] = ta.rsi(df['close'], length=14)
    
    # Simple Moving Averages
    if len(df) >= 50:
        df['sma_50'] = ta.sma(df['close'], length=50)
    else:
        df['sma_50'] = None
        
    if len(df) >= 200:
        df['sma_200'] = ta.sma(df['close'], length=200)
    else:
        df['sma_200'] = None
    
    # MACD
    macd = ta.macd(df['close'])
    if macd is not None and not macd.empty:
        df['macd'] = macd['MACD_12_26_9']
        df['macd_signal'] = macd['MACDs_12_26_9']
    else:
        df['macd'] = None
        df['macd_signal'] = None
    
    # ATR (14-day)
    df['atr_14'] = ta.atr(df['high'], df['low'], df['close'], length=14)
    
    # Volume average (20-day)
    if len(df) >= 20:
        df['volume_avg_20'] = ta.sma(df['volume'], length=20)
    else:
        df['volume_avg_20'] = None
    
    return df


def store_indicators(ticker: str, df: pd.DataFrame) -> bool:
    """Store calculated indicators in database.
    
    Args:
        ticker: Stock ticker symbol
        df: DataFrame with indicator columns
        
    Returns:
        True if storage successful
    """
    if df.empty:
        print(f"Warning: No data to store for {ticker}", file=sys.stderr)
        return False
    
    # Get the most recent row with indicators
    latest = df.iloc[-1]
    
    try:
        with get_database_connection() as conn:
            with conn.cursor() as cur:
                insert_query = """
                    INSERT INTO technical_indicators 
                    (ticker, timestamp, rsi_14, sma_50, sma_200, macd, macd_signal, atr_14, volume_avg_20, created_at)
                    VALUES (%s, %s, %s, %s, %s, %s, %s, %s, %s, %s)
                """
                
                # Helper to safely convert values
                def safe_float(val):
                    if pd.isna(val):
                        return None
                    return float(val)
                
                def safe_int(val):
                    if pd.isna(val):
                        return None
                    return int(val)
                
                cur.execute(insert_query, (
                    ticker,
                    latest['timestamp'] if pd.notna(latest.get('timestamp')) else datetime.now(),
                    safe_float(latest.get('rsi_14')),
                    safe_float(latest.get('sma_50')),
                    safe_float(latest.get('sma_200')),
                    safe_float(latest.get('macd')),
                    safe_float(latest.get('macd_signal')),
                    safe_float(latest.get('atr_14')),
                    safe_int(latest.get('volume_avg_20')),
                    datetime.now()
                ))
                
                conn.commit()
                return True
        
    except Exception as e:
        print(f"Error storing indicators for {ticker}: {e}", file=sys.stderr)
        return False


def get_ticker_list() -> list[str]:
    """Get list of tickers to process.
    
    Returns:
        List of ticker symbols
    """
    # Check command line args first
    if len(sys.argv) > 1:
        return [t.strip().upper() for t in sys.argv[1].split(',')]
    
    # Fall back to environment variable
    tickers_str = os.getenv('TRACKED_TICKERS', 'SPY,QQQ,VOO,VTI')
    return [t.strip().upper() for t in tickers_str.split(',')]


def main() -> None:
    """Main entry point for indicator calculation."""
    try:
        tickers = get_ticker_list()
        results = []
        errors = []
        
        for ticker in tickers:
            print(f"Calculating indicators for {ticker}...", file=sys.stderr)
            
            # Fetch market data
            df = fetch_market_data(ticker)
            
            if df.empty:
                errors.append(f"No market data for {ticker}")
                continue
            
            # Calculate indicators
            df = calculate_indicators(df)
            
            # Store in database
            if store_indicators(ticker, df):
                # Get latest values for output
                latest = df.iloc[-1]
                results.append({
                    'ticker': ticker,
                    'rsi_14': round(float(latest['rsi_14']), 2) if pd.notna(latest.get('rsi_14')) else None,
                    'sma_50': round(float(latest['sma_50']), 2) if pd.notna(latest.get('sma_50')) else None,
                    'sma_200': round(float(latest['sma_200']), 2) if pd.notna(latest.get('sma_200')) else None,
                    'macd': round(float(latest['macd']), 4) if pd.notna(latest.get('macd')) else None,
                    'atr_14': round(float(latest['atr_14']), 4) if pd.notna(latest.get('atr_14')) else None,
                })
                print(f"✓ Stored indicators for {ticker}", file=sys.stderr)
            else:
                errors.append(f"Failed to store indicators for {ticker}")
        
        # Output JSON result
        output = {
            'status': 'success' if not errors else 'partial',
            'processed': len(results),
            'errors': errors,
            'indicators': results,
            'timestamp': datetime.now().isoformat()
        }
        
        print(json.dumps(output, indent=2))
        
        if errors and not results:
            sys.exit(1)
            
    except Exception as e:
        print(json.dumps({
            'status': 'error',
            'message': str(e),
            'timestamp': datetime.now().isoformat()
        }), file=sys.stderr)
        sys.exit(1)


if __name__ == '__main__':
    main()
