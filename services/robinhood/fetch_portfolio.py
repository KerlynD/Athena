#!/usr/bin/env python3
"""
Robinhood Portfolio Fetcher for Market Intelligence Aggregator.

Fetches current portfolio holdings from Robinhood and stores them in the database.
Outputs JSON to stdout for consumption by Go orchestrator.

Usage:
    python fetch_portfolio.py

Environment Variables Required:
    DATABASE_URL: PostgreSQL connection string
    ROBINHOOD_USERNAME: Robinhood account email
    ROBINHOOD_PASSWORD: Robinhood account password
    ROBINHOOD_TOTP: (Optional) TOTP secret for 2FA
"""

import os
import sys
import json
from datetime import datetime

try:
    import robin_stocks.robinhood as rh
    import psycopg2
    from psycopg2.extras import execute_values
except ImportError as e:
    print(json.dumps({
        'status': 'error',
        'message': f'Missing dependency: {e}. Run: pip install -r requirements.txt'
    }))
    sys.exit(1)


def validate_env() -> None:
    """Validate required environment variables are set."""
    required = ['DATABASE_URL', 'ROBINHOOD_USERNAME', 'ROBINHOOD_PASSWORD']
    missing = [var for var in required if not os.getenv(var)]
    if missing:
        raise ValueError(f"Missing required environment variables: {', '.join(missing)}")


def login() -> dict:
    """Authenticate with Robinhood.
    
    Returns:
        Login result dict from robin_stocks
        
    Raises:
        Exception: If login fails
    """
    username = os.getenv('ROBINHOOD_USERNAME')
    password = os.getenv('ROBINHOOD_PASSWORD')
    totp = os.getenv('ROBINHOOD_TOTP')

    # Build login kwargs
    login_kwargs = {
        'username': username,
        'password': password,
    }
    
    # Add TOTP if provided (for 2FA accounts)
    if totp:
        login_kwargs['mfa_code'] = totp

    login_result = rh.login(**login_kwargs)
    
    if not login_result:
        raise Exception("Robinhood login failed - check credentials")
    
    return login_result


def fetch_holdings() -> list[dict]:
    """Fetch current portfolio holdings from Robinhood.
    
    Returns:
        List of holding dictionaries with ticker, quantity, avg_cost, etc.
    """
    positions = rh.account.build_holdings()
    holdings_data = []

    for ticker, data in positions.items():
        try:
            holdings_data.append({
                'ticker': ticker,
                'quantity': float(data.get('quantity', 0)),
                'avg_cost': float(data.get('average_buy_price', 0)),
                'current_price': float(data.get('price', 0)),
                'market_value': float(data.get('equity', 0))
            })
        except (ValueError, TypeError) as e:
            print(f"Warning: Could not parse data for {ticker}: {e}", file=sys.stderr)
            continue

    return holdings_data


def store_holdings(holdings: list[dict]) -> int:
    """Store holdings in the database.
    
    Clears existing holdings and inserts new ones in a transaction.
    
    Args:
        holdings: List of holding dictionaries
        
    Returns:
        Number of holdings stored
    """
    conn = None
    cur = None
    
    try:
        conn = psycopg2.connect(os.getenv('DATABASE_URL'))
        cur = conn.cursor()

        # Clear existing holdings (this is a full refresh)
        cur.execute("DELETE FROM holdings")

        if holdings:
            # Insert new holdings
            insert_query = """
                INSERT INTO holdings (ticker, quantity, avg_cost, current_price, market_value, updated_at)
                VALUES %s
            """
            values = [
                (
                    h['ticker'],
                    h['quantity'],
                    h['avg_cost'],
                    h['current_price'],
                    h['market_value'],
                    datetime.now()
                )
                for h in holdings
            ]
            
            execute_values(cur, insert_query, values)

        conn.commit()
        return len(holdings)
        
    finally:
        if cur:
            cur.close()
        if conn:
            conn.close()


def main() -> None:
    """Main entry point for portfolio fetching."""
    try:
        # Validate environment
        validate_env()

        # Login to Robinhood
        login()

        # Fetch current holdings
        holdings = fetch_holdings()

        # Store in database
        count = store_holdings(holdings)

        # Calculate portfolio summary
        total_value = sum(h['market_value'] for h in holdings)
        total_cost = sum(h['avg_cost'] * h['quantity'] for h in holdings)
        total_gain = total_value - total_cost
        gain_percent = (total_gain / total_cost * 100) if total_cost > 0 else 0

        # Output JSON for Go orchestrator
        result = {
            'status': 'success',
            'holdings_count': count,
            'total_value': round(total_value, 2),
            'total_cost': round(total_cost, 2),
            'total_gain': round(total_gain, 2),
            'gain_percent': round(gain_percent, 2),
            'holdings': holdings,
            'timestamp': datetime.now().isoformat()
        }
        
        print(json.dumps(result, indent=2))

    except Exception as e:
        error_result = {
            'status': 'error',
            'message': str(e),
            'timestamp': datetime.now().isoformat()
        }
        print(json.dumps(error_result), file=sys.stderr)
        sys.exit(1)
    
    finally:
        # Logout from Robinhood
        try:
            rh.logout()
        except Exception:
            pass


if __name__ == '__main__':
    main()
