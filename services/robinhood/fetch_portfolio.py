#!/usr/bin/env python3
"""
Robinhood Portfolio Fetcher for Market Intelligence Aggregator.

Fetches current portfolio holdings from Robinhood and stores them in the database.
Outputs JSON to stdout for consumption by Go orchestrator.

Usage:
    python fetch_portfolio.py [--account-type roth|individual|all]

Environment Variables Required:
    DATABASE_URL: PostgreSQL connection string
    ROBINHOOD_USERNAME: Robinhood account email
    ROBINHOOD_PASSWORD: Robinhood account password
    ROBINHOOD_TOTP: (Optional) TOTP secret for 2FA
    ROBINHOOD_ACCOUNT_TYPE: (Optional) 'roth', 'individual', or 'all'. Defaults to 'all'
"""

import os
import sys
import json
import argparse
import traceback
from datetime import datetime

try:
    import robin_stocks.robinhood as rh
    import psycopg
except ImportError as e:
    print(json.dumps({
        'status': 'error',
        'message': f'Missing dependency: {e}. Run: pip install -r requirements.txt'
    }))
    sys.exit(1)


def log(msg: str) -> None:
    """Log to stderr so it doesn't pollute JSON output."""
    print(msg, file=sys.stderr)


def validate_env() -> None:
    """Validate required environment variables are set."""
    required = ['DATABASE_URL', 'ROBINHOOD_USERNAME', 'ROBINHOOD_PASSWORD']
    missing = [var for var in required if not os.getenv(var)]
    if missing:
        raise ValueError(f"Missing required environment variables: {', '.join(missing)}")


def login() -> dict:
    """Authenticate with Robinhood with session caching.
    
    Uses store_session=True to cache credentials and avoid repeated verification.
    """
    username = os.getenv('ROBINHOOD_USERNAME')
    password = os.getenv('ROBINHOOD_PASSWORD')
    totp = os.getenv('ROBINHOOD_TOTP')

    log("Logging in to Robinhood...")

    login_kwargs = {
        'username': username,
        'password': password,
        'store_session': True,
        'expiresIn': 86400,
    }
    
    if totp:
        login_kwargs['mfa_code'] = totp

    login_result = rh.login(**login_kwargs)
    
    if not login_result:
        raise Exception("Robinhood login failed - check credentials")
    
    log("Login successful!")
    return login_result


def get_account_info() -> dict:
    """Get information about all accounts."""
    try:
        # Get basic account info
        account_profile = rh.profiles.load_account_profile()
        log(f"Account profile loaded")
        
        # Try to get all accounts
        accounts = {}
        
        # Check for retirement accounts
        try:
            # This endpoint lists all linked accounts
            all_accounts = rh.account.load_phoenix_account()
            if all_accounts:
                log(f"Phoenix account data: {json.dumps(all_accounts, indent=2)[:500]}")
                accounts['phoenix'] = all_accounts
        except Exception as e:
            log(f"Phoenix account not available: {e}")
        
        return accounts
    except Exception as e:
        log(f"Error getting account info: {e}")
        return {}


def fetch_all_positions() -> list[dict]:
    """Fetch ALL positions from all accounts including retirement accounts.
    
    Uses multiple methods to ensure we get both individual and IRA holdings.
    """
    holdings_data = []
    seen_tickers = set()  # Avoid duplicates
    
    log("Fetching all positions...")
    
    # Method 1: build_holdings - gets individual account holdings
    try:
        log("Trying build_holdings() for individual account...")
        positions = rh.account.build_holdings()
        
        if positions:
            log(f"Found {len(positions)} positions via build_holdings")
            for ticker, data in positions.items():
                try:
                    holdings_data.append({
                        'ticker': ticker,
                        'quantity': float(data.get('quantity', 0)),
                        'avg_cost': float(data.get('average_buy_price', 0)),
                        'current_price': float(data.get('price', 0)),
                        'market_value': float(data.get('equity', 0)),
                        'account_type': 'individual'
                    })
                    seen_tickers.add(ticker)
                except (ValueError, TypeError) as e:
                    log(f"Warning: Could not parse {ticker}: {e}")
                    continue
    except Exception as e:
        log(f"build_holdings failed: {e}")
    
    # Method 2: Try to get IRA/retirement positions via get_all_positions
    # This endpoint sometimes includes retirement positions
    try:
        log("Trying get_all_positions() for all accounts...")
        all_positions = rh.account.get_all_positions()
        log(f"Found {len(all_positions)} total positions")
        
        for position in all_positions:
            try:
                quantity = float(position.get('quantity', 0))
                if quantity <= 0:
                    continue
                
                instrument_url = position.get('instrument')
                if not instrument_url:
                    continue
                
                instrument = rh.stocks.get_instrument_by_url(instrument_url)
                ticker = instrument.get('symbol', 'UNKNOWN')
                
                # Check account URL to determine type
                account_url = position.get('account', '')
                
                # Determine if this is a retirement account
                # Retirement accounts have different URL patterns
                is_retirement = False
                account_type = 'individual'
                
                if 'retirement' in account_url.lower():
                    is_retirement = True
                    account_type = 'roth'
                elif '/accounts/' in account_url:
                    # Check the account number format
                    # Individual accounts typically have different ID patterns
                    pass
                
                # Skip if we already have this from individual account
                # unless this is a retirement position
                key = f"{ticker}_{account_type}"
                if key in seen_tickers and not is_retirement:
                    continue
                
                quote = rh.stocks.get_latest_price(ticker)
                current_price = float(quote[0]) if quote and quote[0] else 0
                
                avg_cost = float(position.get('average_buy_price', 0))
                market_value = quantity * current_price
                
                holdings_data.append({
                    'ticker': ticker,
                    'quantity': quantity,
                    'avg_cost': avg_cost,
                    'current_price': current_price,
                    'market_value': round(market_value, 2),
                    'account_type': account_type
                })
                seen_tickers.add(key)
            except Exception as e:
                log(f"Warning: Could not parse position: {e}")
                continue
    except Exception as e:
        log(f"get_all_positions failed: {e}")
    
    # Method 3: Try retirement-specific endpoints
    log("Trying retirement-specific endpoints...")
    try:
        # Try to access retirement account data directly
        # Note: robin_stocks may not have full IRA support
        
        # Get profile to find account IDs
        profile_data = rh.profiles.load_account_profile()
        if profile_data:
            log(f"Profile data keys: {list(profile_data.keys())}")
            
        # Try getting IRA positions through alternative endpoint
        ira_holdings = fetch_ira_positions()
        if ira_holdings:
            log(f"Found {len(ira_holdings)} IRA positions")
            for h in ira_holdings:
                key = f"{h['ticker']}_roth"
                if key not in seen_tickers:
                    holdings_data.append(h)
                    seen_tickers.add(key)
    except Exception as e:
        log(f"Retirement endpoints failed: {e}")
    
    log(f"Total holdings found: {len(holdings_data)}")
    return holdings_data


def fetch_ira_positions() -> list[dict]:
    """Attempt to fetch IRA positions using alternative methods."""
    ira_holdings = []
    
    # Method 1: Try the standard accounts endpoint
    try:
        retirement_url = "https://api.robinhood.com/accounts/"
        
        response = rh.globals.SESSION.get(retirement_url)
        if response.status_code == 200:
            accounts_data = response.json()
            log(f"Accounts response: {json.dumps(accounts_data, default=str)[:500]}")
            
            results = accounts_data.get('results', [])
            for account in results:
                account_type = account.get('type', '')
                account_url = account.get('url', '')
                
                log(f"Found account: type={account_type}, url={account_url}")
                
                if 'retirement' in account_type.lower() or 'ira' in account_type.lower():
                    positions_url = account.get('positions', '')
                    if positions_url:
                        pos_response = rh.globals.SESSION.get(positions_url)
                        if pos_response.status_code == 200:
                            positions_data = pos_response.json()
                            for pos in positions_data.get('results', []):
                                quantity = float(pos.get('quantity', 0))
                                if quantity > 0:
                                    instrument_url = pos.get('instrument')
                                    instrument = rh.stocks.get_instrument_by_url(instrument_url)
                                    ticker = instrument.get('symbol', 'UNKNOWN')
                                    
                                    quote = rh.stocks.get_latest_price(ticker)
                                    current_price = float(quote[0]) if quote and quote[0] else 0
                                    
                                    ira_holdings.append({
                                        'ticker': ticker,
                                        'quantity': quantity,
                                        'avg_cost': float(pos.get('average_buy_price', 0)),
                                        'current_price': current_price,
                                        'market_value': round(quantity * current_price, 2),
                                        'account_type': 'roth'
                                    })
    except Exception as e:
        log(f"Standard accounts endpoint failed: {e}")
    
    # Method 2: Try retirement-specific endpoints
    if not ira_holdings:
        retirement_endpoints = [
            "https://api.robinhood.com/retirement/accounts/",
            "https://api.robinhood.com/ira/accounts/",
            "https://bonfire.robinhood.com/accounts/",
        ]
        
        for endpoint in retirement_endpoints:
            try:
                log(f"Trying endpoint: {endpoint}")
                response = rh.globals.SESSION.get(endpoint)
                log(f"  Status: {response.status_code}")
                
                if response.status_code == 200:
                    data = response.json()
                    log(f"  Response: {json.dumps(data, default=str)[:300]}")
                    
                    # Try to extract accounts/positions from the response
                    accounts = data.get('results', []) or data.get('accounts', []) or [data]
                    
                    for account in accounts:
                        if isinstance(account, dict):
                            # Try to get positions URL
                            positions_url = account.get('positions') or account.get('positions_url')
                            if positions_url:
                                log(f"  Found positions URL: {positions_url}")
                                pos_response = rh.globals.SESSION.get(positions_url)
                                if pos_response.status_code == 200:
                                    parse_positions_into_holdings(pos_response.json(), ira_holdings)
            except Exception as e:
                log(f"  Endpoint {endpoint} failed: {e}")
    
    return ira_holdings


def parse_positions_into_holdings(positions_data: dict, holdings_list: list) -> None:
    """Parse positions response and add to holdings list."""
    positions = positions_data.get('results', []) or positions_data.get('positions', []) or []
    
    for pos in positions:
        try:
            quantity = float(pos.get('quantity', 0))
            if quantity <= 0:
                continue
            
            instrument_url = pos.get('instrument')
            if not instrument_url:
                continue
            
            instrument = rh.stocks.get_instrument_by_url(instrument_url)
            ticker = instrument.get('symbol', 'UNKNOWN')
            
            quote = rh.stocks.get_latest_price(ticker)
            current_price = float(quote[0]) if quote and quote[0] else 0
            
            holdings_list.append({
                'ticker': ticker,
                'quantity': quantity,
                'avg_cost': float(pos.get('average_buy_price', 0)),
                'current_price': current_price,
                'market_value': round(quantity * current_price, 2),
                'account_type': 'roth'
            })
            log(f"  Found IRA position: {ticker} x {quantity}")
        except Exception as e:
            log(f"  Could not parse position: {e}")


def filter_by_account_type(holdings: list[dict], account_type: str) -> list[dict]:
    """Filter holdings by account type."""
    if account_type == 'all':
        return holdings
    
    filtered = [h for h in holdings if h.get('account_type', '').lower() == account_type.lower()]
    log(f"Filtered to {len(filtered)} {account_type} holdings")
    return filtered


def store_holdings(holdings: list[dict]) -> int:
    """Store holdings in the database."""
    db_url = os.getenv('DATABASE_URL')
    
    try:
        with psycopg.connect(db_url) as conn:
            with conn.cursor() as cur:
                # Clear existing holdings
                cur.execute("DELETE FROM holdings")

                if holdings:
                    insert_query = """
                        INSERT INTO holdings (ticker, quantity, avg_cost, current_price, market_value, updated_at)
                        VALUES (%s, %s, %s, %s, %s, %s)
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
                    
                    cur.executemany(insert_query, values)

                conn.commit()
                log(f"Stored {len(holdings)} holdings in database")
                return len(holdings)
    except Exception as e:
        log(f"Database error: {e}")
        log(traceback.format_exc())
        raise


def main() -> None:
    """Main entry point for portfolio fetching."""
    parser = argparse.ArgumentParser(description='Fetch Robinhood portfolio')
    parser.add_argument('--account-type', choices=['roth', 'individual', 'all'], 
                        default=os.getenv('ROBINHOOD_ACCOUNT_TYPE', 'all'),
                        help='Account type to fetch (default: all)')
    parser.add_argument('--debug', action='store_true',
                        help='Enable debug output')
    args = parser.parse_args()
    
    try:
        # Validate environment
        validate_env()

        # Login to Robinhood
        login()
        
        # Debug: show account info
        if args.debug:
            get_account_info()

        # Fetch all holdings
        holdings = fetch_all_positions()
        
        # Filter by account type if specified
        if args.account_type != 'all':
            holdings = filter_by_account_type(holdings, args.account_type)

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
            'account_type': args.account_type,
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
        log(f"Error: {e}")
        log(traceback.format_exc())
        error_result = {
            'status': 'error',
            'message': str(e),
            'timestamp': datetime.now().isoformat()
        }
        print(json.dumps(error_result))
        sys.exit(1)


if __name__ == '__main__':
    main()
